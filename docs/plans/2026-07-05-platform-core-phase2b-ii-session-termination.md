# Platform Core — Фаза 2b-ii (Завершение сессий + auth API) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Дать пользователю возможность видеть и завершать свои сессии: завершить конкретную (по устройству) и «выйти везде», через аутентифицированный (Bearer-токен) JSON API поверх token-introspection Hydra.

**Architecture:** Расширяем `HydraClient` порт методами revoke (по subject / по sid) и introspection. Use-cases `ListSessions`/`TerminateSession`/`TerminateAllSessions` (с проверкой владения). HTTP: middleware `RequireAuth` валидирует `Authorization: Bearer <token>` через Hydra introspection и кладёт user id в контекст; поверх — эндпоинты `/api/sessions`. Завершение = revoke в Hydra (SSO рвётся) + пометка `ended_at` у нас.

**Scope note:** 2b-ii = backend завершения сессий + Bearer-auth API. НЕ входит (→ 2c): аккаунт-хаб UI (список/кнопки), браузерная аутентификация хаба, register/reset HTML, RP-initiated logout через Hydra logout-флоу.

**Tech Stack:** Go 1.26, chi, `github.com/ory/hydra-client-go/v2` v2.2.1, testify, testcontainers (не нужны здесь — всё на фейках/httptest).

---

## Предпосылки
- 2b-i дал: `identity.HydraClient` (GetLoginRequest/AcceptLoginRequest/RejectLoginRequest/GetConsentRequest/AcceptConsentRequest/RejectConsentRequest), `identity.SessionRepository` (Create/FindByID/ListActiveByUser/MarkEnded/MarkEndedByHydraSID/MarkAllEndedByUser), Postgres session-repo, реальный Ory-клиент, DI. Config, chi router, wire.
- Фейки в `presentation/http/fakes_test.go`: `fakeUsers`, `fakeHasher`, `fakeTokens`, `fakeMailer`, `fakeHydra` (реализует HydraClient), `fakeSessions` (реализует SessionRepository).

---

## File Structure (эта фаза)

```
platform/internal/
  domain/identity/
    hydra.go              (+RevokeLoginSessionsBySubject, +RevokeLoginSessionByID, +IntrospectToken)
  application/identity/
    sessions_admin.go     ListSessions/TerminateSession/TerminateAllSessions + sessions_admin_test.go
  infrastructure/hydra/
    client.go             (+реализация 3 новых методов; latitude по Ory SDK)
  presentation/http/
    auth_middleware.go    RequireAuth (introspection) + auth_middleware_test.go
    session_handlers.go   GET/DELETE/POST /api/sessions + session_handlers_test.go
  infrastructure/di/wire.go   (+провайдеры + mount)
```

---

## Task 1: Расширить HydraClient порт (revoke + introspect) + реальная реализация

**Files:**
- Modify: `platform/internal/domain/identity/hydra.go`
- Modify: `platform/internal/infrastructure/hydra/client.go`
- Modify: `platform/internal/presentation/http/fakes_test.go` (дополнить fakeHydra)

- [ ] **Step 1: Добавить методы в порт** — в `platform/internal/domain/identity/hydra.go` в интерфейс `HydraClient` добавить:
```go
	// RevokeLoginSessionsBySubject terminates ALL of a subject's login sessions ("logout everywhere").
	RevokeLoginSessionsBySubject(ctx context.Context, subject string) error
	// RevokeLoginSessionByID terminates a single Hydra login session by its sid.
	RevokeLoginSessionByID(ctx context.Context, sid string) error
	// IntrospectToken validates an access token; returns active and the subject (user id).
	IntrospectToken(ctx context.Context, token string) (active bool, subject string, err error)
```

- [ ] **Step 2: Реализовать в реальном клиенте** — в `platform/internal/infrastructure/hydra/client.go` добавить методы. Intended shape (адаптировать под Ory SDK v2.2.1 — метод `RevokeOAuth2LoginSessions` принимает query-параметры `subject`/`sid`; `IntrospectOAuth2Token` возвращает `IntrospectedOAuth2Token` с `Active` и `Sub`):
```go
func (c *Client) RevokeLoginSessionsBySubject(ctx context.Context, subject string) error {
	_, err := c.api.OAuth2API.RevokeOAuth2LoginSessions(ctx).Subject(subject).Execute()
	if err != nil {
		return fmt.Errorf("hydra: revoke sessions by subject: %w", err)
	}
	return nil
}

func (c *Client) RevokeLoginSessionByID(ctx context.Context, sid string) error {
	_, err := c.api.OAuth2API.RevokeOAuth2LoginSessions(ctx).Sid(sid).Execute()
	if err != nil {
		return fmt.Errorf("hydra: revoke session by sid: %w", err)
	}
	return nil
}

func (c *Client) IntrospectToken(ctx context.Context, token string) (bool, string, error) {
	res, _, err := c.api.OAuth2API.IntrospectOAuth2Token(ctx).Token(token).Execute()
	if err != nil {
		return false, "", fmt.Errorf("hydra: introspect token: %w", err)
	}
	if !res.Active {
		return false, "", nil
	}
	subject := ""
	if res.Sub != nil {
		subject = *res.Sub
	}
	return true, subject, nil
}
```
Если имена методов/полей SDK иные — открыть module cache, поправить, сохранив сигнатуры порта. Compile-time assertion `var _ identity.HydraClient = (*Client)(nil)` уже есть — он проверит полноту.

- [ ] **Step 3: Дополнить fakeHydra** — в `platform/internal/presentation/http/fakes_test.go` у `fakeHydra` добавить поля и методы:
```go
// add fields to fakeHydra struct:
//   revokedSubject string
//   revokedSID     string
//   introspectActive  bool
//   introspectSubject string

func (f *fakeHydra) RevokeLoginSessionsBySubject(_ context.Context, subject string) error {
	f.revokedSubject = subject
	return nil
}
func (f *fakeHydra) RevokeLoginSessionByID(_ context.Context, sid string) error {
	f.revokedSID = sid
	return nil
}
func (f *fakeHydra) IntrospectToken(_ context.Context, _ string) (bool, string, error) {
	return f.introspectActive, f.introspectSubject, nil
}
```
(Add the 4 fields to the `fakeHydra` struct definition.)

- [ ] **Step 4: Проверить сборку**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go build ./... && go test -short ./internal/presentation/http/`
Expected: build чисто; существующие http-тесты проходят (fakeHydra снова реализует полный интерфейс).

- [ ] **Step 5: Commit** (`go mod tidy` if needed)

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/domain/identity/hydra.go platform/internal/infrastructure/hydra/ platform/internal/presentation/http/fakes_test.go
git commit -m "feat(platform): hydra revoke + introspect methods"
```

---

## Task 2: Use-cases ListSessions / TerminateSession / TerminateAllSessions

**Files:**
- Create: `platform/internal/application/identity/sessions_admin.go`
- Test: `platform/internal/application/identity/sessions_admin_test.go`

**Логика владения:** `TerminateSession(userID, sessionID)` находит сессию; если `session.UserID != userID` → возвращает `ErrSessionNotFound` (не раскрываем чужие). Revoke в Hydra по `HydraSessionID`, затем `MarkEnded`.

- [ ] **Step 1: Написать падающий тест** — `platform/internal/application/identity/sessions_admin_test.go`:
```go
package identity_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
)

func TestListSessions(t *testing.T) {
	sessions := newFakeSessionRepo()
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "u1", HydraSessionID: "sid1"})
	uc := identity.NewListSessions(sessions)

	got, err := uc.Execute(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "s1", got[0].ID)
}

func TestTerminateSessionRevokesAndEnds(t *testing.T) {
	sessions := newFakeSessionRepo()
	hydra := &fakeHydraAdmin{}
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "u1", HydraSessionID: "sid1"})
	uc := identity.NewTerminateSession(sessions, hydra)

	require.NoError(t, uc.Execute(context.Background(), "u1", "s1"))
	require.Equal(t, "sid1", hydra.revokedSID)
	require.True(t, sessions.ended["s1"])
}

func TestTerminateSessionRejectsOtherUsersSession(t *testing.T) {
	sessions := newFakeSessionRepo()
	hydra := &fakeHydraAdmin{}
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "OWNER", HydraSessionID: "sid1"})
	uc := identity.NewTerminateSession(sessions, hydra)

	err := uc.Execute(context.Background(), "ATTACKER", "s1")
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
	require.Empty(t, hydra.revokedSID)          // no revoke
	require.False(t, sessions.ended["s1"])       // not ended
}

func TestTerminateAllSessions(t *testing.T) {
	sessions := newFakeSessionRepo()
	hydra := &fakeHydraAdmin{}
	uc := identity.NewTerminateAllSessions(sessions, hydra)

	require.NoError(t, uc.Execute(context.Background(), "u1"))
	require.Equal(t, "u1", hydra.revokedSubject)
	require.True(t, sessions.allEnded["u1"])
}
```

- [ ] **Step 2: Добавить фейки для этого пакета** — `platform/internal/application/identity/sessions_admin_fakes_test.go`:
```go
package identity_test

import (
	"context"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

// fakeSessionRepo implements domain.SessionRepository for use-case tests.
type fakeSessionRepo struct {
	byID     map[string]*domain.Session
	ended    map[string]bool
	allEnded map[string]bool
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{byID: map[string]*domain.Session{}, ended: map[string]bool{}, allEnded: map[string]bool{}}
}
func (f *fakeSessionRepo) Create(_ context.Context, s *domain.Session) error {
	cp := *s
	f.byID[s.ID] = &cp
	return nil
}
func (f *fakeSessionRepo) FindByID(_ context.Context, id string) (*domain.Session, error) {
	if s, ok := f.byID[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, domain.ErrSessionNotFound
}
func (f *fakeSessionRepo) ListActiveByUser(_ context.Context, userID string) ([]*domain.Session, error) {
	var out []*domain.Session
	for _, s := range f.byID {
		if s.UserID == userID && !f.ended[s.ID] {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (f *fakeSessionRepo) MarkEnded(_ context.Context, id string) error { f.ended[id] = true; return nil }
func (f *fakeSessionRepo) MarkEndedByHydraSID(_ context.Context, _ string) error { return nil }
func (f *fakeSessionRepo) MarkAllEndedByUser(_ context.Context, userID string) error {
	f.allEnded[userID] = true
	return nil
}

// fakeHydraAdmin implements the subset of domain.HydraClient used by session use-cases.
// It embeds a no-op for the login/consent methods so it satisfies the full interface.
type fakeHydraAdmin struct {
	revokedSubject string
	revokedSID     string
}

func (f *fakeHydraAdmin) GetLoginRequest(context.Context, string) (*domain.HydraLoginRequest, error) {
	return &domain.HydraLoginRequest{}, nil
}
func (f *fakeHydraAdmin) AcceptLoginRequest(context.Context, string, string, bool) (string, error) {
	return "", nil
}
func (f *fakeHydraAdmin) RejectLoginRequest(context.Context, string, string) (string, error) {
	return "", nil
}
func (f *fakeHydraAdmin) GetConsentRequest(context.Context, string) (*domain.HydraConsentRequest, error) {
	return &domain.HydraConsentRequest{}, nil
}
func (f *fakeHydraAdmin) AcceptConsentRequest(context.Context, string, []string) (string, error) {
	return "", nil
}
func (f *fakeHydraAdmin) RejectConsentRequest(context.Context, string, string) (string, error) {
	return "", nil
}
func (f *fakeHydraAdmin) RevokeLoginSessionsBySubject(_ context.Context, subject string) error {
	f.revokedSubject = subject
	return nil
}
func (f *fakeHydraAdmin) RevokeLoginSessionByID(_ context.Context, sid string) error {
	f.revokedSID = sid
	return nil
}
func (f *fakeHydraAdmin) IntrospectToken(context.Context, string) (bool, string, error) {
	return false, "", nil
}
```

- [ ] **Step 3: Запустить — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/application/identity/ -run "TestListSessions|TestTerminate" -v`
Expected: FAIL (нет конструкторов).

- [ ] **Step 4: Реализовать** — `platform/internal/application/identity/sessions_admin.go`:
```go
package identity

import (
	"context"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

// ListSessions returns a user's active sessions.
type ListSessions struct {
	sessions domain.SessionRepository
}

func NewListSessions(sessions domain.SessionRepository) *ListSessions {
	return &ListSessions{sessions: sessions}
}

func (uc *ListSessions) Execute(ctx context.Context, userID string) ([]*domain.Session, error) {
	return uc.sessions.ListActiveByUser(ctx, userID)
}

// TerminateSession ends one of the user's own sessions (revokes it in Hydra too).
type TerminateSession struct {
	sessions domain.SessionRepository
	hydra    domain.HydraClient
}

func NewTerminateSession(sessions domain.SessionRepository, hydra domain.HydraClient) *TerminateSession {
	return &TerminateSession{sessions: sessions, hydra: hydra}
}

func (uc *TerminateSession) Execute(ctx context.Context, userID, sessionID string) error {
	s, err := uc.sessions.FindByID(ctx, sessionID)
	if err != nil {
		return err // ErrSessionNotFound
	}
	if s.UserID != userID {
		return domain.ErrSessionNotFound // ownership: don't reveal others' sessions
	}
	if s.HydraSessionID != "" {
		if err := uc.hydra.RevokeLoginSessionByID(ctx, s.HydraSessionID); err != nil {
			return err
		}
	}
	return uc.sessions.MarkEnded(ctx, sessionID)
}

// TerminateAllSessions ends every session of the user ("logout everywhere").
type TerminateAllSessions struct {
	sessions domain.SessionRepository
	hydra    domain.HydraClient
}

func NewTerminateAllSessions(sessions domain.SessionRepository, hydra domain.HydraClient) *TerminateAllSessions {
	return &TerminateAllSessions{sessions: sessions, hydra: hydra}
}

func (uc *TerminateAllSessions) Execute(ctx context.Context, userID string) error {
	if err := uc.hydra.RevokeLoginSessionsBySubject(ctx, userID); err != nil {
		return err
	}
	return uc.sessions.MarkAllEndedByUser(ctx, userID)
}
```

- [ ] **Step 5: Запустить — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/application/identity/ -v`
Expected: PASS (все, включая новые).

- [ ] **Step 6: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/application/identity/
git commit -m "feat(platform): session termination use-cases"
```

---

## Task 3: Auth middleware (Bearer introspection)

**Files:**
- Create: `platform/internal/presentation/http/auth_middleware.go`
- Test: `platform/internal/presentation/http/auth_middleware_test.go`

**Логика:** извлечь `Authorization: Bearer <token>`; `IntrospectToken`; если не active/пусто → 401; иначе положить subject (user id) в контекст запроса и вызвать next.

- [ ] **Step 1: Написать падающий тест** — `platform/internal/presentation/http/auth_middleware_test.go`:
```go
package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	mw := apphttp.RequireAuth(&fakeHydra{})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireAuthRejectsInactiveToken(t *testing.T) {
	hydra := &fakeHydra{introspectActive: false}
	mw := apphttp.RequireAuth(hydra)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireAuthPassesAndSetsUser(t *testing.T) {
	hydra := &fakeHydra{introspectActive: true, introspectSubject: "user-42"}
	mw := apphttp.RequireAuth(hydra)

	var seen string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = apphttp.UserIDFromContext(r.Context())
		w.WriteHeader(200)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer good")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "user-42", seen)
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/presentation/http/ -run TestRequireAuth -v`
Expected: FAIL (нет `RequireAuth`/`UserIDFromContext`).

- [ ] **Step 3: Реализовать** — `platform/internal/presentation/http/auth_middleware.go`:
```go
package http

import (
	"context"
	"net/http"
	"strings"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

type ctxKey int

const userIDKey ctxKey = 0

// RequireAuth validates a Bearer access token via Hydra introspection and puts
// the subject (user id) into the request context.
func RequireAuth(hydra domain.HydraClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(authz, prefix) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			token := strings.TrimSpace(authz[len(prefix):])
			active, subject, err := hydra.IntrospectToken(r.Context(), token)
			if err != nil {
				http.Error(w, "auth error", http.StatusBadGateway)
				return
			}
			if !active || subject == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey, subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext returns the authenticated user id, or "" if none.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}
```

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/presentation/http/ -run TestRequireAuth -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/presentation/http/auth_middleware.go platform/internal/presentation/http/auth_middleware_test.go
git commit -m "feat(platform): bearer-token auth middleware via introspection"
```

---

## Task 4: Session-management API (/api/sessions)

**Files:**
- Create: `platform/internal/presentation/http/session_handlers.go`
- Test: `platform/internal/presentation/http/session_handlers_test.go`

**Эндпоинты (под RequireAuth):**
- `GET /api/sessions` → список активных сессий текущего юзера (JSON).
- `DELETE /api/sessions/{id}` → завершить конкретную (владение) → 204.
- `POST /api/sessions/logout-all` → завершить все → 204.

- [ ] **Step 0: Проапгрейдить `fakeSessions` до рабочего in-memory стора** (обязательно!)

The `fakeSessions` created in Phase 2b-i has stub methods (`FindByID` always returns `ErrSessionNotFound`, `ListActiveByUser` returns `created`). The DELETE test needs a working `FindByID`. In `platform/internal/presentation/http/fakes_test.go`, REPLACE the `fakeSessions` type and its methods with a real in-memory store (keep the `created` slice so the existing 2b-i consent test still passes):
```go
type fakeSessions struct {
	created []*identity.Session
	byID    map[string]*identity.Session
	ended   map[string]bool
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{byID: map[string]*identity.Session{}, ended: map[string]bool{}}
}
func (f *fakeSessions) Create(_ context.Context, s *identity.Session) error {
	if f.byID == nil {
		f.byID = map[string]*identity.Session{}
		f.ended = map[string]bool{}
	}
	cp := *s
	f.created = append(f.created, &cp)
	f.byID[s.ID] = &cp
	return nil
}
func (f *fakeSessions) FindByID(_ context.Context, id string) (*identity.Session, error) {
	if s, ok := f.byID[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, identity.ErrSessionNotFound
}
func (f *fakeSessions) ListActiveByUser(_ context.Context, userID string) ([]*identity.Session, error) {
	var out []*identity.Session
	for _, s := range f.created {
		if s.UserID == userID && !f.ended[s.ID] {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (f *fakeSessions) MarkEnded(_ context.Context, id string) error { f.ended[id] = true; return nil }
func (f *fakeSessions) MarkEndedByHydraSID(_ context.Context, _ string) error { return nil }
func (f *fakeSessions) MarkAllEndedByUser(_ context.Context, userID string) error {
	for _, s := range f.created {
		if s.UserID == userID {
			f.ended[s.ID] = true
		}
	}
	return nil
}
```
NOTE: the 2b-i consent handler test constructs `&fakeSessions{}` directly and asserts `sessions.created` — that still works (zero-value struct; `Create` lazily inits maps). Keep those call sites compiling; do not rename `fakeSessions`.

- [ ] **Step 1: Написать падающий тест** — `platform/internal/presentation/http/session_handlers_test.go`:
```go
package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	appidentity "github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

func buildSessionAPI(t *testing.T, userID string) (*httptest.Server, *fakeSessions, *fakeHydra) {
	t.Helper()
	sessions := &fakeSessions{}
	hydra := &fakeHydra{introspectActive: true, introspectSubject: userID}
	h := apphttp.NewSessionHandlers(
		appidentity.NewListSessions(sessions),
		appidentity.NewTerminateSession(sessions, hydra),
		appidentity.NewTerminateAllSessions(sessions, hydra),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireAuth(hydra))
		h.Register(pr)
	})
	return httptest.NewServer(r), sessions, hydra
}

func TestListSessionsEndpoint(t *testing.T) {
	srv, sessions, _ := buildSessionAPI(t, "u1")
	defer srv.Close()
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "u1", HydraSessionID: "sid1", DeviceName: "Chrome"})

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Len(t, out, 1)
	require.Equal(t, "s1", out[0]["id"])
}

func TestDeleteSessionEndpoint(t *testing.T) {
	srv, sessions, hydra := buildSessionAPI(t, "u1")
	defer srv.Close()
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "u1", HydraSessionID: "sid1"})

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/sessions/s1", nil)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, "sid1", hydra.revokedSID)
}

func TestLogoutAllEndpoint(t *testing.T) {
	srv, _, hydra := buildSessionAPI(t, "u1")
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/sessions/logout-all", nil)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, "u1", hydra.revokedSubject)
}

func TestSessionAPIRequiresAuth(t *testing.T) {
	srv, _, _ := buildSessionAPI(t, "u1")
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/sessions") // no token
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
```
(NOTE: this reuses `fakeSessions` from Phase 2b-i's `fakes_test.go`; that fake's `ListActiveByUser` returns `f.created`. That's fine for these assertions.)

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/presentation/http/ -run "TestListSessionsEndpoint|TestDeleteSession|TestLogoutAll|TestSessionAPIRequiresAuth" -v`
Expected: FAIL (нет `NewSessionHandlers`).

- [ ] **Step 3: Реализовать** — `platform/internal/presentation/http/session_handlers.go`:
```go
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	appidentity "github.com/papyrus/platform/internal/application/identity"
)

// SessionHandlers exposes the authenticated session-management API.
type SessionHandlers struct {
	list         *appidentity.ListSessions
	terminate    *appidentity.TerminateSession
	terminateAll *appidentity.TerminateAllSessions
}

func NewSessionHandlers(list *appidentity.ListSessions, terminate *appidentity.TerminateSession,
	terminateAll *appidentity.TerminateAllSessions) *SessionHandlers {
	return &SessionHandlers{list: list, terminate: terminate, terminateAll: terminateAll}
}

// Register mounts the /api/sessions routes (expects RequireAuth applied by caller).
func (h *SessionHandlers) Register(r chi.Router) {
	r.Get("/api/sessions", h.listSessions)
	r.Delete("/api/sessions/{id}", h.deleteSession)
	r.Post("/api/sessions/logout-all", h.logoutAll)
}

func (h *SessionHandlers) listSessions(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	sessions, err := h.list.Execute(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	type dto struct {
		ID         string `json:"id"`
		DeviceName string `json:"device_name"`
		IP         string `json:"ip"`
		Current    bool   `json:"current"`
	}
	out := make([]dto, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, dto{ID: s.ID, DeviceName: s.DeviceName, IP: s.IP})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *SessionHandlers) deleteSession(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.terminate.Execute(r.Context(), userID, id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SessionHandlers) logoutAll(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	if err := h.terminateAll.Execute(r.Context(), userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```
(`writeJSON` already exists in `identity_handlers.go` in this package — reuse it.)

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/presentation/http/ -v`
Expected: PASS (все http-тесты).

- [ ] **Step 5: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/presentation/http/session_handlers.go platform/internal/presentation/http/session_handlers_test.go
git commit -m "feat(platform): session-management API (/api/sessions)"
```

---

## Task 5: DI-проводка + mount под RequireAuth

**Files:**
- Modify: `platform/internal/infrastructure/di/wire.go` (+ regen)
- Modify: `platform/internal/infrastructure/httpserver/server.go`

- [ ] **Step 1: Провайдер SessionHandlers** — в `wire.go` добавить:
```go
func provideSessionHandlers(sessions domainidentity.SessionRepository, hydraClient domainidentity.HydraClient) *apphttp.SessionHandlers {
	return apphttp.NewSessionHandlers(
		appidentity.NewListSessions(sessions),
		appidentity.NewTerminateSession(sessions, hydraClient),
		appidentity.NewTerminateAllSessions(sessions, hydraClient),
	)
}
```
Обновить `provideServer`, чтобы принимал `*apphttp.SessionHandlers` и `hydraClient` (для middleware) — см. Step 2. Добавить `provideSessionHandlers` в `wire.Build(...)`.

- [ ] **Step 2: Обновить NewRouter + provideServer** — `httpserver/server.go`:
```go
func NewRouter(identity *apphttp.IdentityHandlers, auth *apphttp.AuthHandlers,
	sessions *apphttp.SessionHandlers, hydra domainidentity.HydraClient) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", apphttp.Healthz())
	identity.Register(r)
	auth.Register(r)

	// Authenticated platform API.
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireAuth(hydra))
		sessions.Register(pr)
	})

	return r
}
```
Добавить импорт `domainidentity "github.com/papyrus/platform/internal/domain/identity"` в server.go.
И `provideServer` в wire.go:
```go
func provideServer(cfg config.Config, identity *apphttp.IdentityHandlers, auth *apphttp.AuthHandlers,
	sessions *apphttp.SessionHandlers, hydra domainidentity.HydraClient) *http.Server {
	return httpserver.NewServer(":"+cfg.Port, httpserver.NewRouter(identity, auth, sessions, hydra))
}
```
Обновить `server_test.go`: вызвать `NewRouter(idh, ah, apphttp.NewSessionHandlers(nil,nil,nil), &noopHydra{})` — где для теста `/healthz` нужен ненулевой `HydraClient` для middleware конструктора (сам middleware не вызывается на `/healthz`, но `RequireAuth(hydra)` строится при сборке роутера). Проще: передать существующий фейк недоступен из `http` (не test-пакет). Поэтому в `server.go`-тесте (`package httpserver`) объяви минимальный no-op, реализующий `domainidentity.HydraClient`, или используй реальный `hydra.New("http://localhost:0", nil)` (он не делает сетевых вызовов при конструировании). **Используй `hydra.New("http://localhost:0", nil)`** — конструктор не ходит в сеть, а `/healthz` не триггерит middleware.

- [ ] **Step 3: Regen wire + сборка + тесты**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && make wire && go build ./... && go vet ./... && go test -short ./...`
Expected: wire regen ок, build/vet чисто, юнит-тесты зелёные.

- [ ] **Step 4: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/infrastructure/di/ platform/internal/infrastructure/httpserver/
git commit -m "feat(platform): wire session-management API under auth"
```

---

## Task 6: Финальная проверка фазы

- [ ] **Step 1: Юнит + полные тесты**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test -short ./... && go test ./...`
Expected: PASS.

- [ ] **Step 2: vet + build**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go vet ./... && go build ./...`
Expected: чисто.

- [ ] **Step 3: E2E smoke (Docker) — опционально, но желательно**

Поднять стек (`docker compose up -d --build --wait`), пройти OAuth-флоу (как в 2b-i Task 9) чтобы получить access token на callback, затем:
```bash
# introspection-защищённый список сессий (нужен валидный access token TOKEN)
curl -sf http://localhost:8090/api/sessions -H "Authorization: Bearer $TOKEN"
# logout-all
curl -sf -X POST http://localhost:8090/api/sessions/logout-all -H "Authorization: Bearer $TOKEN" -w '%{http_code}\n'
```
Получение реального access token требует обмена authorization code на токен (token endpoint Hydra, public client с PKCE). Если это в smoke флаки — достаточно юнит/интеграционного покрытия; пометь, что E2E токен-флоу отложен. Затем `docker compose down`.

---

## Definition of Done (Фаза 2b-ii)
- `HydraClient` умеет revoke по subject и по sid + introspection; реальная реализация на Ory SDK.
- Use-cases: список сессий, завершение своей сессии (с проверкой владения — чужую нельзя), «выйти везде».
- Middleware `RequireAuth`: Bearer-токен → introspection → user id в контексте; без/невалидный токен → 401.
- API `/api/sessions` (GET/DELETE/{id}/POST logout-all) под RequireAuth, покрыт httptest.
- Все тесты зелёные; vet/build чистые.

## Открытый вопрос (к 2c)
Как браузерный аккаунт-хаб (server-rendered) аутентифицирует пользователя для вызова `/api/sessions`: стать OAuth-клиентом Hydra и носить токен, либо читать Hydra login-сессию. Решаем в 2c.

## Следующая фаза
Фаза 2c: аккаунт-хаб UI (профиль, свитчер воркспейсов, список/завершение сессий, register/reset HTML) + браузерная аутентификация хаба.
