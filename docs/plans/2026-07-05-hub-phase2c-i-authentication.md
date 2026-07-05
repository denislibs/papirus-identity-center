# Account Hub — Фаза 2c-i (Аутентификация хаба) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Сделать аккаунт-хаб знающим текущего пользователя: хаб — OIDC-клиент Hydra, проходит OAuth-танец, кладёт server-side cookie-сессию (Redis), и защищённые страницы хаба видят user id.

**Architecture:** Хаб — это тот же сервис `platform-core`, но выступающий ещё и как confidential OIDC-клиент Hydra. Поток: `/account` без сессии → `/auth/login` → Hydra authorize (наш login/consent из 2b) → `/auth/callback` → обмен кода на токен (x/oauth2) → introspection → subject (= наш user id) → server-side hub-сессия в Redis, cookie с opaque id → страницы хаба. Шаг «code → subject» спрятан за портом `CodeExchanger` (реальный: oauth2 exchange + Hydra introspect; фейк для тестов). Профиль читается из `UserRepository.FindByID(sub)` — хаб in-process, HTTP-токены не нужны.

**Scope note (2c-i):** OIDC-аутентификация хаба + cookie-сессия + middleware + минимальная страница профиля + logout. НЕ входит (→ 2c-ii): страница управления сессиями (UI), register/reset HTML, свитчер воркспейсов (нет Workspace-модуля).

**Tech Stack:** Go 1.26, chi, html/template, `golang.org/x/oauth2`, Redis (go-redis), Ory Hydra (introspection через существующий HydraClient), testify, testcontainers.

**Module:** `github.com/denislibs/papirus-identity-center`

---

## Предпосылки
- Identity готов: `identity.UserRepository.FindByID`, `HydraClient.IntrospectToken`, login/consent-флоу (2b-i), config (`Hydra.PublicURL`, `Hydra.AdminURL`, `BaseURL`, `Port`, `TrustedClientIDs`). Redis `Connect`. DI (wire). chi router (`NewRouter`).

---

## File Structure (эта фаза)

```
internal/
  config/config.go                (+HubClientID/HubClientSecret/SelfURL; +test)
  domain/identity/hub.go          порт CodeExchanger + тип HubSession
  infrastructure/
    hubauth/exchanger.go          реальный CodeExchanger (oauth2 + introspect)
    redis/hub_session_store.go    server-side hub-сессии + hub_session_store_test.go
  presentation/http/
    hub_auth_handlers.go          /auth/login /auth/callback /auth/logout + test
    hub_middleware.go             RequireHubSession + HubUserIDFromContext + test
    hub_handlers.go               GET /account (профиль) + test
    templates/account.html        страница профиля
  cmd/bootstrap-client/main.go    (+регистрация hub OIDC-клиента)
  infrastructure/di/wire.go       (+провайдеры, +mount)
```

---

## Task 1: Порт CodeExchanger + тип HubSession + конфиг хаба

**Files:**
- Create: `internal/domain/identity/hub.go`
- Modify: `internal/config/config.go` (+ test)

- [ ] **Step 1: Домен** — `internal/domain/identity/hub.go`:
```go
package identity

import "context"

// CodeExchanger exchanges an OAuth2 authorization code for the authenticated
// user's subject (which equals the platform user id).
type CodeExchanger interface {
	ExchangeForSubject(ctx context.Context, code string) (subject string, err error)
}

// HubSessionStore persists server-side hub browser sessions (keyed by opaque id).
type HubSessionStore interface {
	// Create stores a new session for the subject, returns the opaque session id.
	Create(ctx context.Context, subject string) (id string, err error)
	// Subject returns the subject bound to the session id, or ErrSessionNotFound.
	Subject(ctx context.Context, id string) (string, error)
	// Delete removes a session (logout).
	Delete(ctx context.Context, id string) error
}
```
(reuses existing `ErrSessionNotFound`.)

- [ ] **Step 2: Конфиг — тест** — add to `internal/config/config_test.go`:
```go
func TestLoadReadsHubConfig(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("DB_HOST", "db"); t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "u"); t.Setenv("DB_PASSWORD", "p"); t.Setenv("DB_NAME", "n")
	t.Setenv("REDIS_HOST", "r"); t.Setenv("REDIS_PORT", "6379")
	t.Setenv("HUB_CLIENT_ID", "hub"); t.Setenv("HUB_CLIENT_SECRET", "secret")
	t.Setenv("SELF_URL", "http://localhost:8090")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "hub", cfg.HubClientID)
	require.Equal(t, "secret", cfg.HubClientSecret)
	require.Equal(t, "http://localhost:8090", cfg.SelfURL)
}
```

- [ ] **Step 3: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/config/ -run TestLoadReadsHubConfig -v` → FAIL.

- [ ] **Step 4: Реализовать конфиг** — в `Config` добавить поля `HubClientID string`, `HubClientSecret string`, `SelfURL string`; в структуру `HydraConfig` добавить поле `TokenURL string` (internal token endpoint, для server-to-server обмена кода — в Docker это `http://hydra:4444`, тогда как `PublicURL` браузерный `http://localhost:4444`). В `Load()` перед `return`:
```go
	cfg.HubClientID = os.Getenv("HUB_CLIENT_ID")
	cfg.HubClientSecret = os.Getenv("HUB_CLIENT_SECRET")
	cfg.SelfURL = os.Getenv("SELF_URL")
	if cfg.SelfURL == "" {
		cfg.SelfURL = "http://localhost:" + cfg.Port
	}
	cfg.Hydra.TokenURL = os.Getenv("HYDRA_TOKEN_URL")
	if cfg.Hydra.TokenURL == "" {
		cfg.Hydra.TokenURL = cfg.Hydra.PublicURL // non-docker local: same host
	}
```
(where `cfg.Hydra` is read earlier in `Load()`; add the `TokenURL` line alongside the existing `HydraConfig` population.)

- [ ] **Step 5: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/config/ -v` → PASS.

- [ ] **Step 6: Проверить сборку домена.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go build ./internal/domain/... && go build ./internal/config/`

- [ ] **Step 7: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/domain/identity/hub.go internal/config/
git commit -m "feat(hub): CodeExchanger/HubSessionStore ports + hub config"
```

---

## Task 2: Redis HubSessionStore

**Files:**
- Create: `internal/infrastructure/redis/hub_session_store.go`
- Test: `internal/infrastructure/redis/hub_session_store_test.go`

- [ ] **Step 1: Падающий интеграционный тест** — `internal/infrastructure/redis/hub_session_store_test.go`:
```go
package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

func TestHubSessionStore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()
	container, err := tcredis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	defer func() { _ = container.Terminate(ctx) }()
	endpoint, err := container.Endpoint(ctx, "")
	require.NoError(t, err)
	client, err := Connect(ctx, endpoint)
	require.NoError(t, err)
	defer client.Close()

	store := NewHubSessionStore(client, time.Hour)

	id, err := store.Create(ctx, "user-1")
	require.NoError(t, err)
	require.NotEmpty(t, id)

	sub, err := store.Subject(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "user-1", sub)

	require.NoError(t, store.Delete(ctx, id))
	_, err = store.Subject(ctx, id)
	require.ErrorIs(t, err, identity.ErrSessionNotFound)
}
```

- [ ] **Step 2: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/infrastructure/redis/ -run TestHubSessionStore -v` → FAIL (нет NewHubSessionStore).

- [ ] **Step 3: Реализовать** — `internal/infrastructure/redis/hub_session_store.go`:
```go
package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// HubSessionStore implements identity.HubSessionStore backed by Redis with TTL.
type HubSessionStore struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewHubSessionStore(client *goredis.Client, ttl time.Duration) *HubSessionStore {
	return &HubSessionStore{client: client, ttl: ttl}
}

func hubKey(id string) string { return "hubsession:" + id }

func (s *HubSessionStore) Create(ctx context.Context, subject string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("redis: gen hub session id: %w", err)
	}
	id := hex.EncodeToString(buf)
	if err := s.client.Set(ctx, hubKey(id), subject, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("redis: store hub session: %w", err)
	}
	return id, nil
}

func (s *HubSessionStore) Subject(ctx context.Context, id string) (string, error) {
	sub, err := s.client.Get(ctx, hubKey(id)).Result()
	if errors.Is(err, goredis.Nil) {
		return "", identity.ErrSessionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("redis: get hub session: %w", err)
	}
	return sub, nil
}

func (s *HubSessionStore) Delete(ctx context.Context, id string) error {
	if err := s.client.Del(ctx, hubKey(id)).Err(); err != nil {
		return fmt.Errorf("redis: delete hub session: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/infrastructure/redis/ -run TestHubSessionStore -v` → PASS.

- [ ] **Step 5: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/redis/
git commit -m "feat(hub): redis hub session store"
```

---

## Task 3: Реальный CodeExchanger (oauth2 + introspection)

**Files:**
- Create: `internal/infrastructure/hubauth/exchanger.go`

**API latitude:** `golang.org/x/oauth2` API is stable; Hydra public endpoints are `<publicURL>/oauth2/auth` and `<publicURL>/oauth2/token`. This task has no unit test (does real HTTP) — verified via the E2E in Task 8. Acceptance: compiles + implements `identity.CodeExchanger`.

- [ ] **Step 1: Реализовать** — `internal/infrastructure/hubauth/exchanger.go`:
```go
package hubauth

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"

	"github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// Exchanger implements identity.CodeExchanger using the OAuth2 code flow against
// Hydra, then resolves the subject via token introspection.
type Exchanger struct {
	oauth      *oauth2.Config
	introspect identity.HydraClient
}

// New builds an Exchanger. authBaseURL is the BROWSER-facing Hydra base (for the
// redirect to /oauth2/auth, e.g. http://localhost:4444); tokenBaseURL is the
// server-reachable base for the token exchange (in Docker http://hydra:4444).
func New(clientID, clientSecret, redirectURL, authBaseURL, tokenBaseURL string, introspect identity.HydraClient) *Exchanger {
	return &Exchanger{
		oauth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "profile"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  authBaseURL + "/oauth2/auth",
				TokenURL: tokenBaseURL + "/oauth2/token",
			},
		},
		introspect: introspect,
	}
}

// AuthCodeURL returns the URL to redirect the browser to, with the given state.
func (e *Exchanger) AuthCodeURL(state string) string {
	return e.oauth.AuthCodeURL(state)
}

// ExchangeForSubject swaps the code for a token and introspects it for the subject.
func (e *Exchanger) ExchangeForSubject(ctx context.Context, code string) (string, error) {
	tok, err := e.oauth.Exchange(ctx, code)
	if err != nil {
		return "", fmt.Errorf("hubauth: exchange code: %w", err)
	}
	active, subject, err := e.introspect.IntrospectToken(ctx, tok.AccessToken)
	if err != nil {
		return "", fmt.Errorf("hubauth: introspect: %w", err)
	}
	if !active || subject == "" {
		return "", fmt.Errorf("hubauth: token inactive")
	}
	return subject, nil
}

var _ identity.CodeExchanger = (*Exchanger)(nil)
```
(NOTE: `AuthCodeURL` is a concrete helper on `*Exchanger`, used by the login handler; the port only requires `ExchangeForSubject`. The handler will depend on a small local interface that includes both — see Task 4.)

- [ ] **Step 2: Сборка.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go build ./... && go vet ./...`
Adapt to the actual x/oauth2 API if needed; `go get golang.org/x/oauth2` if not already direct, `go mod tidy`.

- [ ] **Step 3: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/hubauth/ go.mod go.sum
git commit -m "feat(hub): oauth2 code exchanger with introspection"
```

---

## Task 4: Hub auth handlers (/auth/login, /auth/callback, /auth/logout)

**Files:**
- Create: `internal/presentation/http/hub_auth_handlers.go`
- Test: `internal/presentation/http/hub_auth_handlers_test.go`

**Логика:**
- `GET /auth/login`: сгенерировать random `state`, положить в короткоживущий cookie `hub_oauth_state`, редирект на `authURL(state)`.
- `GET /auth/callback?code=&state=`: сверить `state` с cookie; `ExchangeForSubject(code)` → subject; `HubSessionStore.Create(subject)` → id; поставить cookie `hub_session` (HttpOnly, SameSite=Lax, Path=/); редирект на `/account`.
- `GET /auth/logout`: прочитать cookie `hub_session`, `HubSessionStore.Delete(id)`, очистить cookie, редирект на `/account` (который отправит на login).

**Интерфейс для тестируемости** (в этом файле):
```go
type HubOAuth interface {
	AuthCodeURL(state string) string
	ExchangeForSubject(ctx context.Context, code string) (string, error)
}
```
(`*hubauth.Exchanger` его удовлетворяет; в тестах — фейк.)

- [ ] **Step 1: Падающий тест** — `internal/presentation/http/hub_auth_handlers_test.go`:
```go
package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

// fakes for hub auth
type fakeHubOAuth struct {
	state       string
	exchangeSub string
	exchangeErr error
}

func (f *fakeHubOAuth) AuthCodeURL(state string) string { f.state = state; return "https://hydra/auth?state=" + state }
func (f *fakeHubOAuth) ExchangeForSubject(_ context.Context, _ string) (string, error) {
	return f.exchangeSub, f.exchangeErr
}

type fakeHubStore struct {
	created string
	deleted string
	id      string
}

func (f *fakeHubStore) Create(_ context.Context, subject string) (string, error) {
	f.created = subject
	if f.id == "" {
		f.id = "hubid-1"
	}
	return f.id, nil
}
func (f *fakeHubStore) Subject(_ context.Context, id string) (string, error) { return f.created, nil }
func (f *fakeHubStore) Delete(_ context.Context, id string) error            { f.deleted = id; return nil }

func newHubAuthServer(t *testing.T) (*httptest.Server, *fakeHubOAuth, *fakeHubStore) {
	t.Helper()
	oauth := &fakeHubOAuth{exchangeSub: "user-1"}
	store := &fakeHubStore{}
	h := apphttp.NewHubAuthHandlers(oauth, store)
	r := chi.NewRouter()
	h.Register(r)
	return httptest.NewServer(r), oauth, store
}

func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func TestHubLoginRedirectsToHydraWithState(t *testing.T) {
	srv, _, _ := newHubAuthServer(t)
	defer srv.Close()
	resp, err := noRedirect().Get(srv.URL + "/auth/login")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Location"), "https://hydra/auth?state="))
	// state cookie set
	require.NotEmpty(t, resp.Cookies())
}

func TestHubCallbackCreatesSessionAndCookie(t *testing.T) {
	srv, oauth, store := newHubAuthServer(t)
	defer srv.Close()

	// first hit /auth/login to obtain a state + cookie
	loginResp, err := noRedirect().Get(srv.URL + "/auth/login")
	require.NoError(t, err)
	loginResp.Body.Close()
	stateCookie := loginResp.Cookies()[0]
	state := oauth.state

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/callback?code=abc&state="+state, nil)
	req.AddCookie(stateCookie)
	resp, err := noRedirect().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/account", resp.Header.Get("Location"))
	require.Equal(t, "user-1", store.created)
	// hub_session cookie set
	var hubCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "hub_session" {
			hubCookie = c
		}
	}
	require.NotNil(t, hubCookie)
	require.Equal(t, "hubid-1", hubCookie.Value)
	require.True(t, hubCookie.HttpOnly)
}

func TestHubCallbackRejectsStateMismatch(t *testing.T) {
	srv, _, store := newHubAuthServer(t)
	defer srv.Close()
	loginResp, _ := noRedirect().Get(srv.URL + "/auth/login")
	loginResp.Body.Close()
	stateCookie := loginResp.Cookies()[0]

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/callback?code=abc&state=WRONG", nil)
	req.AddCookie(stateCookie)
	resp, err := noRedirect().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Empty(t, store.created) // no session created
}
```

- [ ] **Step 2: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run TestHub -v` → FAIL (нет NewHubAuthHandlers).

- [ ] **Step 3: Реализовать** — `internal/presentation/http/hub_auth_handlers.go`:
```go
package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/go-chi/chi/v5"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// HubOAuth is the OAuth client behavior the hub needs.
type HubOAuth interface {
	AuthCodeURL(state string) string
	ExchangeForSubject(ctx context.Context, code string) (string, error)
}

const (
	stateCookie   = "hub_oauth_state"
	sessionCookie = "hub_session"
)

// HubAuthHandlers implement the hub's OIDC-client login/callback/logout.
type HubAuthHandlers struct {
	oauth HubOAuth
	store domain.HubSessionStore
}

func NewHubAuthHandlers(oauth HubOAuth, store domain.HubSessionStore) *HubAuthHandlers {
	return &HubAuthHandlers{oauth: oauth, store: store}
}

func (h *HubAuthHandlers) Register(r chi.Router) {
	r.Get("/auth/login", h.login)
	r.Get("/auth/callback", h.callback)
	r.Get("/auth/logout", h.logout)
}

func randToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *HubAuthHandlers) login(w http.ResponseWriter, r *http.Request) {
	state := randToken()
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 300,
	})
	http.Redirect(w, r, h.oauth.AuthCodeURL(state), http.StatusFound)
}

func (h *HubAuthHandlers) callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(stateCookie)
	if err != nil || c.Value == "" || c.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	subject, err := h.oauth.ExchangeForSubject(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "login failed", http.StatusBadGateway)
		return
	}
	id, err := h.store.Create(r.Context(), subject)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	// clear state cookie, set session cookie
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: id, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/account", http.StatusFound)
}

func (h *HubAuthHandlers) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = h.store.Delete(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/account", http.StatusFound)
}
```

- [ ] **Step 4: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run TestHub -v` → PASS.

- [ ] **Step 5: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/presentation/http/hub_auth_handlers.go internal/presentation/http/hub_auth_handlers_test.go
git commit -m "feat(hub): OIDC-client login/callback/logout handlers"
```

---

## Task 5: Hub session middleware

**Files:**
- Create: `internal/presentation/http/hub_middleware.go`
- Test: `internal/presentation/http/hub_middleware_test.go`

**Логика:** прочитать cookie `hub_session`; `HubSessionStore.Subject(id)`; если нет/невалидна → редирект на `/auth/login`; иначе положить subject в контекст.

- [ ] **Step 1: Падающий тест** — `internal/presentation/http/hub_middleware_test.go`:
```go
package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

func TestRequireHubSessionRedirectsWhenNoCookie(t *testing.T) {
	store := &fakeHubStore{}
	mw := apphttp.RequireHubSession(store)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/auth/login", rec.Header().Get("Location"))
}

func TestRequireHubSessionPassesAndSetsUser(t *testing.T) {
	store := &fakeHubStore{created: "user-9", id: "hubid-9"}
	mw := apphttp.RequireHubSession(store)
	var seen string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = apphttp.HubUserIDFromContext(r.Context())
		w.WriteHeader(200)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	req.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-9"})
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "user-9", seen)
}
```
(NOTE: `fakeHubStore.Subject` returns `f.created` regardless of id — fine for these tests; set `created` to the expected subject.)

- [ ] **Step 2: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run TestRequireHubSession -v` → FAIL.

- [ ] **Step 3: Реализовать** — `internal/presentation/http/hub_middleware.go`:
```go
package http

import (
	"context"
	"net/http"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

type hubCtxKey int

const hubUserKey hubCtxKey = 0

// RequireHubSession loads the hub session from the cookie; redirects to /auth/login
// if absent/invalid, else puts the subject (user id) into context.
func RequireHubSession(store domain.HubSessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(sessionCookie)
			if err != nil || c.Value == "" {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}
			subject, err := store.Subject(r.Context(), c.Value)
			if err != nil || subject == "" {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), hubUserKey, subject)))
		})
	}
}

// HubUserIDFromContext returns the hub-authenticated user id, or "".
func HubUserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(hubUserKey).(string); ok {
		return v
	}
	return ""
}
```
(NOTE: the `fakeHubStore.Subject` in tests returns `f.created` for any id; the real store returns ErrSessionNotFound for unknown ids, which the middleware treats as redirect. For the "no cookie" test, `fakeHubStore` isn't consulted.)

- [ ] **Step 4: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run TestRequireHubSession -v` → PASS.

- [ ] **Step 5: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/presentation/http/hub_middleware.go internal/presentation/http/hub_middleware_test.go
git commit -m "feat(hub): hub session middleware"
```

---

## Task 6: Страница профиля (GET /account)

**Files:**
- Create: `internal/presentation/http/templates/account.html`
- Create: `internal/presentation/http/hub_handlers.go`
- Test: `internal/presentation/http/hub_handlers_test.go`

- [ ] **Step 1: Шаблон** — `internal/presentation/http/templates/account.html`:
```html
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Account — Papyrus</title></head>
<body>
  <h1>Account</h1>
  <p>Email: {{.Email}}</p>
  <p>Name: {{.Name}}</p>
  <p><a href="/auth/logout">Log out</a></p>
</body>
</html>
```
(NOTE: `MustLoadTemplates` already globs `templates/*.html`, so `account.html` is picked up automatically.)

- [ ] **Step 2: Падающий тест** — `internal/presentation/http/hub_handlers_test.go`:
```go
package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

func TestAccountPageRendersProfile(t *testing.T) {
	users := newFakeUsers()
	_ = users.Create(context.Background(), &domain.User{ID: "user-9", Email: "me@x.com", Name: "Me"})
	store := &fakeHubStore{created: "user-9", id: "hubid-9"}

	h := apphttp.NewHubHandlers(appidentity.NewGetProfile(users), apphttp.MustLoadTemplates())
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireHubSession(store))
		h.Register(pr)
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/account", nil)
	req.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-9"})
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	out := string(body[:n])
	require.True(t, strings.Contains(out, "me@x.com"))
	require.True(t, strings.Contains(out, "Me"))
}
```

- [ ] **Step 3: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run TestAccountPage -v` → FAIL (нет NewHubHandlers / NewGetProfile).

- [ ] **Step 4: Реализовать use-case GetProfile** — `internal/application/identity/get_profile.go`:
```go
package identity

import (
	"context"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// GetProfile fetches a user's profile by id.
type GetProfile struct {
	users domain.UserRepository
}

func NewGetProfile(users domain.UserRepository) *GetProfile {
	return &GetProfile{users: users}
}

func (uc *GetProfile) Execute(ctx context.Context, userID string) (*domain.User, error) {
	return uc.users.FindByID(ctx, userID)
}
```

- [ ] **Step 5: Реализовать hub handlers** — `internal/presentation/http/hub_handlers.go`:
```go
package http

import (
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
)

// HubHandlers render the authenticated account hub pages.
type HubHandlers struct {
	profile *appidentity.GetProfile
	tpl     *template.Template
}

func NewHubHandlers(profile *appidentity.GetProfile, tpl *template.Template) *HubHandlers {
	return &HubHandlers{profile: profile, tpl: tpl}
}

// Register mounts hub pages (expects RequireHubSession applied by caller).
func (h *HubHandlers) Register(r chi.Router) {
	r.Get("/account", h.account)
}

func (h *HubHandlers) account(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	u, err := h.profile.Execute(r.Context(), userID)
	if err != nil {
		http.Error(w, "profile error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tpl.ExecuteTemplate(w, "account.html", map[string]any{"Email": u.Email, "Name": u.Name})
}
```

- [ ] **Step 6: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run TestAccountPage -v` → PASS.

- [ ] **Step 7: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/application/identity/get_profile.go internal/presentation/http/hub_handlers.go internal/presentation/http/hub_handlers_test.go internal/presentation/http/templates/account.html
git commit -m "feat(hub): account profile page"
```

---

## Task 7: DI-проводка + mount + регистрация hub-клиента + env

**Files:**
- Modify: `internal/infrastructure/di/wire.go` (+ regen)
- Modify: `internal/infrastructure/httpserver/server.go`
- Modify: `cmd/bootstrap-client/main.go` (+ hub client)
- Modify: `.env.example`, `docker-compose.yml`

- [ ] **Step 1: Провайдеры в wire.go** — добавить:
```go
func provideHubStore(client *goredis.Client) domainidentity.HubSessionStore {
	return rdc.NewHubSessionStore(client, 24*time.Hour)
}

func provideHubOAuth(cfg config.Config, hydraClient domainidentity.HydraClient) apphttp.HubOAuth {
	return hubauth.New(cfg.HubClientID, cfg.HubClientSecret, cfg.SelfURL+"/auth/callback",
		cfg.Hydra.PublicURL, cfg.Hydra.TokenURL, hydraClient)
}

func provideHubAuthHandlers(oauth apphttp.HubOAuth, store domainidentity.HubSessionStore) *apphttp.HubAuthHandlers {
	return apphttp.NewHubAuthHandlers(oauth, store)
}

func provideHubHandlers(users domainidentity.UserRepository) *apphttp.HubHandlers {
	return apphttp.NewHubHandlers(appidentity.NewGetProfile(users), apphttp.MustLoadTemplates())
}
```
Обновить `provideServer` — добавить параметры `hubAuth *apphttp.HubAuthHandlers`, `hub *apphttp.HubHandlers`, `hubStore domainidentity.HubSessionStore`; передать в `NewRouter`. Добавить новые провайдеры в `wire.Build(...)`; добавить импорты `time`, `hubauth`.

- [ ] **Step 2: NewRouter** — `internal/infrastructure/httpserver/server.go`, расширить сигнатуру и смонтировать:
```go
func NewRouter(identity *apphttp.IdentityHandlers, auth *apphttp.AuthHandlers,
	sessions *apphttp.SessionHandlers, hydra domainidentity.HydraClient,
	hubAuth *apphttp.HubAuthHandlers, hub *apphttp.HubHandlers, hubStore domainidentity.HubSessionStore) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", apphttp.Healthz())
	identity.Register(r)
	auth.Register(r)
	hubAuth.Register(r) // /auth/login, /auth/callback, /auth/logout (public)

	// Bearer API
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireAuth(hydra))
		sessions.Register(pr)
	})
	// Hub pages (cookie session)
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireHubSession(hubStore))
		hub.Register(pr)
	})

	return r
}
```
Обновить `server_test.go`: добавить в вызов `NewRouter` — `apphttp.NewHubAuthHandlers(nil, nil)`, `apphttp.NewHubHandlers(nil, apphttp.MustLoadTemplates())`, и для `hubStore` — передать `redis.NewHubSessionStore(nil, time.Hour)` **нельзя** (nil client вызовет панику только при обращении; `/healthz` не в группе hub, поэтому middleware не сработает). Используй `redis.NewHubSessionStore(nil, time.Hour)` — конструктор не обращается к клиенту. Добавь импорты `time`, `redis` infra.

- [ ] **Step 3: Регистрация hub-клиента в Hydra** — расширить `cmd/bootstrap-client/main.go`: помимо `papyrus`, зарегистрировать confidential-клиент `hub` с `token_endpoint_auth_method: client_secret_post`, redirect `SELF_URL/auth/callback` (env `HUB_REDIRECT_URI`, default `http://localhost:8090/auth/callback`), scope `openid profile`, известным секретом (env `HUB_CLIENT_SECRET`, default `hub-secret`). Idempotent (409 → ok). Адаптировать под Ory SDK.

- [ ] **Step 4: Env** — добавить в `.env.example` и в `docker-compose.yml` (platform-core env):
```
HUB_CLIENT_ID=hub
HUB_CLIENT_SECRET=hub-secret
SELF_URL=http://localhost:8090
HYDRA_TOKEN_URL=http://hydra:4444
```
И **исправить** в `docker-compose.yml` у platform-core существующую строку `HYDRA_PUBLIC_URL` на браузерную: `HYDRA_PUBLIC_URL=http://localhost:4444` (сейчас там `http://hydra:4444` — она годилась, пока PublicURL нигде не использовался как browser-facing; теперь хаб редиректит браузер на неё, поэтому нужен `localhost`). Убедиться, что у сервиса `hydra` `URLS_SELF_ISSUER=http://localhost:4444` (уже так). Внутренний обмен кода пойдёт на `HYDRA_TOKEN_URL=http://hydra:4444`.

- [ ] **Step 5: Regen wire + сборка + тесты**

Run: `cd /Users/denisurevic/Downloads/ББД/platform && make wire && go build ./... && go vet ./... && go test -short ./...`
Expected: всё чисто/зелёно.

- [ ] **Step 6: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/di/ internal/infrastructure/httpserver/ cmd/bootstrap-client/ .env.example docker-compose.yml
git commit -m "feat(hub): wire hub auth + pages, register hub oidc client"
```

---

## Task 8: E2E — браузерная аутентификация хаба

- [ ] **Step 1: Поднять стек + зарегистрировать клиентов**

Run: `cd /Users/denisurevic/Downloads/ББД/platform && docker compose up -d --build --wait`
Затем: `HYDRA_ADMIN_URL=http://localhost:4445 HUB_CLIENT_SECRET=hub-secret go run ./cmd/bootstrap-client`

- [ ] **Step 2: Создать верифицированного пользователя**
```bash
curl -sf -X POST http://localhost:8090/register -H 'Content-Type: application/json' -d '{"email":"hub@example.com","password":"long-enough-pw","name":"Hub User"}'
docker exec platform-postgres psql -U platform -d platform -c "UPDATE users SET email_verified=true WHERE email='hub@example.com';"
```

- [ ] **Step 3: Пройти хаб-флоу с cookie-jar**

С `curl -c/-b /tmp/hubjar.txt -L` начать с `http://localhost:8090/account` → редирект `/auth/login` → Hydra authorize → наш `/login` (POST email/пароль) → consent (авто) → `/auth/callback` → редирект `/account`. В конце `/account` должен вернуть 200 и HTML с `hub@example.com` и `Hub User`. Реализуй последовательность (извлекая login_challenge и state как в 2b-i Task 9). **Acceptance:** финальный GET `/account` (с cookie `hub_session` из jar) отдаёт профиль пользователя.
Проверить сессию хаба в Redis: `docker exec platform-redis redis-cli KEYS 'hubsession:*'` → есть ключ.
Затем `docker compose down`.

Если curl-флоу флаки — допустимо доказать через маленький throwaway Go с `cookiejar.Jar` (не коммитить). Главное — доказать, что `/account` показывает профиль после логина.

- [ ] **Step 4: Commit** (если были правки по ходу E2E — иначе пропустить)

---

## Task 9: Финальная проверка фазы

- [ ] **Step 1:** `cd /Users/denisurevic/Downloads/ББД/platform && go test -short ./... && go test ./...` → PASS.
- [ ] **Step 2:** `go vet ./... && go build ./...` → чисто.
- [ ] **Step 3:** `git push origin main` (запушить фазу в репозиторий).

---

## Definition of Done (Фаза 2c-i)
- Хаб — OIDC-клиент Hydra: `/auth/login` → OAuth → `/auth/callback` создаёт server-side hub-сессию (Redis) + cookie.
- `RequireHubSession` middleware: cookie → subject в контексте, иначе редирект на login.
- `GET /account` (под hub-сессией) рендерит профиль (email/name) из `users`.
- `/auth/logout` завершает hub-сессию.
- State-параметр проверяется на callback (защита от CSRF на login).
- Юнит-тесты (хендлеры/middleware на фейках) + интеграционный (hub-store на реальном Redis) + E2E (браузерный флоу) зелёные; vet/build чисто.

## Открытые вопросы / долги (к 2c-ii и далее)
- Cookie `Secure`-флаг: сейчас для localhost http выключен — включить в проде (по env).
- RP-initiated logout в Hydra (завершать и SSO-сессию Hydra при hub-logout), а не только hub-cookie.
- 4 minor-долга из ревью 2b-ii (неатомарность revoke+mark, 404-vs-502, поле `current`).

## Следующая фаза
2c-ii: страница управления сессиями (список/завершить/выйти везде, дёргает use-cases in-process по hub-subject), register/reset HTML-формы. Затем — Workspace-модуль (воркспейсы, оргструктура, продукты) и свитчер в хабе.
