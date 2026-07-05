# Platform Core — Фаза 2b-i (Hydra login/consent flow + сессии) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Реализовать вход через Ory Hydra: браузер идёт в Hydra `/authorize` → Hydra редиректит на наш login → мы проверяем креды → принимаем login/consent → создаётся сессия с богатой инфой; SSO работает.

**Architecture:** Наш Go-сервис — login/consent provider для Hydra. Взаимодействие с Hydra admin-API инкапсулировано за портом `HydraClient` (реализация — официальный Ory SDK `hydra-client-go/v2`), что позволяет TDD-ить хендлеры на фейке. Login-экран — server-rendered `html/template` (embedded). Сессии хранятся в Postgres с богатой инфой (устройство/IP/UA) и связью с Hydra `sid`. Consent для наших продуктов — авто-принятие (trusted), без HTML.

**Scope note:** 2b-i = login/consent FLOW + login HTML + СОЗДАНИЕ сессии. НЕ входит (→ 2b-ii): завершение сессий (terminate-one/logout-everywhere), logout-эндпоинт. НЕ входит (→ 2c): register/reset HTML, аккаунт-хаб, список сессий UI, consent HTML для сторонних клиентов.

**Tech Stack:** Go 1.26, chi, pgx/v5, html/template (embed), `github.com/ory/hydra-client-go/v2`, testify, testcontainers, Ory Hydra v2.2.0 (уже в docker-compose).

---

## Предпосылки (из предыдущих фаз)
- Есть `identity` домен: `User`, `UserRepository`, `PasswordHasher`, ошибки. Есть Postgres/Redis, DI (wire), config с `Hydra.AdminURL`/`Hydra.PublicURL` и `BaseURL`. Hydra и hydra-migrate уже в `platform/docker-compose.yml`; `URLS_LOGIN=http://localhost:8090/login`, `URLS_CONSENT=http://localhost:8090/consent`.
- Миграции прогоняются на старте (`postgres.RunMigrations`).

---

## File Structure (эта фаза)

```
platform/internal/
  domain/identity/
    session.go            сущность Session + SessionRepository (порт)
    hydra.go              HydraClient (порт) + DTO (LoginRequest/ConsentRequest/ClientInfo)
    errors.go             (+ ErrInvalidCredentials, ErrEmailNotVerified, ErrSessionNotFound)
  application/identity/
    authenticate.go       + authenticate_test.go   (проверка email+пароль)
  infrastructure/
    postgres/
      migrations/0002_sessions.up.sql
      migrations/0002_sessions.down.sql
      session_repository.go        + session_repository_test.go
    hydra/
      client.go           реальный HydraClient на Ory SDK  (latitude по SDK API)
  presentation/http/
    templates/login.html            server-rendered форма логина
    templates.go                    embed + рендерер
    auth_handlers.go                login (GET/POST) + consent (GET) хендлеры
    auth_handlers_test.go           TDD на фейках (HydraClient + repos)
  cmd/bootstrap-client/main.go      one-shot: регистрирует trusted OAuth-клиент в Hydra (для E2E/dev)
```

Modified: `di/wire.go` (+regen), `config.go` (доверенные клиенты — опц.), `httpserver/server.go` (mount auth routes).

---

## Task 1: Миграция sessions

**Files:**
- Create: `platform/internal/infrastructure/postgres/migrations/0002_sessions.up.sql`
- Create: `platform/internal/infrastructure/postgres/migrations/0002_sessions.down.sql`

- [ ] **Step 1: Написать SQL**

`0002_sessions.up.sql`:
```sql
CREATE TABLE sessions (
    id               UUID PRIMARY KEY,
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    hydra_session_id TEXT NOT NULL DEFAULT '',
    device_name      TEXT NOT NULL DEFAULT '',
    user_agent       TEXT NOT NULL DEFAULT '',
    ip               TEXT NOT NULL DEFAULT '',
    location         TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at         TIMESTAMPTZ
);
CREATE INDEX idx_sessions_user ON sessions (user_id) WHERE ended_at IS NULL;
CREATE INDEX idx_sessions_hydra ON sessions (hydra_session_id);
```

`0002_sessions.down.sql`:
```sql
DROP TABLE sessions;
```

- [ ] **Step 2: Проверить, что миграция применяется (расширить существующий тест)**

The migration runner already has `TestRunMigrationsCreatesUsers`. Add a sibling assertion file OR extend: add a new test in `platform/internal/infrastructure/postgres/migrate_test.go`:
```go
func TestRunMigrationsCreatesSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("platform"),
		tcpostgres.WithUsername("platform"),
		tcpostgres.WithPassword("platform"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	require.NoError(t, err)
	defer func() { _ = container.Terminate(ctx) }()

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, RunMigrations(dsn))

	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	var exists bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name='sessions')`).Scan(&exists))
	require.True(t, exists)
}
```

- [ ] **Step 3: Запустить — убедиться, что проходит** (миграция подхватывается embed автоматически)

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/postgres/ -run TestRunMigrationsCreatesSessions -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/infrastructure/postgres/migrations/ platform/internal/infrastructure/postgres/migrate_test.go
git commit -m "feat(platform): sessions migration"
```

---

## Task 2: Домен — Session, SessionRepository, HydraClient (порты) + ошибки

**Files:**
- Create: `platform/internal/domain/identity/session.go`
- Create: `platform/internal/domain/identity/hydra.go`
- Modify: `platform/internal/domain/identity/errors.go`

- [ ] **Step 1: Session + SessionRepository** — `platform/internal/domain/identity/session.go`:
```go
package identity

import (
	"context"
	"time"
)

// Session is a rich record of an authenticated browser session, linked to a
// Hydra login session (HydraSessionID = Hydra "sid").
type Session struct {
	ID             string
	UserID         string
	HydraSessionID string
	DeviceName     string
	UserAgent      string
	IP             string
	Location       string
	CreatedAt      time.Time
	LastSeenAt     time.Time
	EndedAt        *time.Time
}

// SessionRepository persists sessions.
type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	FindByID(ctx context.Context, id string) (*Session, error) // ErrSessionNotFound if absent
	ListActiveByUser(ctx context.Context, userID string) ([]*Session, error)
	MarkEnded(ctx context.Context, id string) error
	MarkEndedByHydraSID(ctx context.Context, sid string) error
	MarkAllEndedByUser(ctx context.Context, userID string) error
}
```

- [ ] **Step 2: HydraClient порт + DTO** — `platform/internal/domain/identity/hydra.go`:
```go
package identity

import "context"

// OAuthClientInfo is the subset of an OAuth2 client we care about.
type OAuthClientInfo struct {
	ID      string
	Name    string
	Trusted bool // our own products → auto-consent
}

// HydraLoginRequest is Hydra's view of a pending login.
type HydraLoginRequest struct {
	Challenge string
	Skip      bool   // Hydra already has an authenticated session
	Subject   string // set when Skip is true
	Client    OAuthClientInfo
}

// HydraConsentRequest is Hydra's view of a pending consent.
type HydraConsentRequest struct {
	Challenge       string
	Skip            bool
	Subject         string
	LoginSessionID  string // Hydra "sid" — stored in our Session
	RequestedScopes []string
	Client          OAuthClientInfo
}

// HydraClient wraps the Ory Hydra admin API operations we use.
type HydraClient interface {
	GetLoginRequest(ctx context.Context, challenge string) (*HydraLoginRequest, error)
	AcceptLoginRequest(ctx context.Context, challenge, subject string, remember bool) (redirectTo string, err error)
	RejectLoginRequest(ctx context.Context, challenge, reason string) (redirectTo string, err error)
	GetConsentRequest(ctx context.Context, challenge string) (*HydraConsentRequest, error)
	AcceptConsentRequest(ctx context.Context, challenge string, grantScopes []string) (redirectTo string, err error)
}
```

- [ ] **Step 3: Добавить ошибки** — в `platform/internal/domain/identity/errors.go` добавить в блок var:
```go
	// ErrInvalidCredentials is returned when email/password don't match.
	ErrInvalidCredentials = errors.New("identity: invalid credentials")
	// ErrEmailNotVerified is returned when a user tries to log in before verifying email.
	ErrEmailNotVerified = errors.New("identity: email not verified")
	// ErrSessionNotFound is returned by SessionRepository lookups when absent.
	ErrSessionNotFound = errors.New("identity: session not found")
```

- [ ] **Step 4: Проверить компиляцию**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go build ./internal/domain/...`
Expected: без ошибок.

- [ ] **Step 5: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/domain/identity/
git commit -m "feat(platform): session + hydra ports, auth errors"
```

---

## Task 3: Postgres SessionRepository

**Files:**
- Create: `platform/internal/infrastructure/postgres/session_repository.go`
- Test: `platform/internal/infrastructure/postgres/session_repository_test.go`

- [ ] **Step 1: Написать падающий интеграционный тест** — `platform/internal/infrastructure/postgres/session_repository_test.go`:
```go
package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/papyrus/platform/internal/domain/identity"
)

func TestSessionRepositoryCreateListEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	userRepo := NewUserRepository(w.pool)
	sessRepo := NewSessionRepository(w.pool)

	// a session references a user (FK), so create the user first
	uid := "33333333-3333-3333-3333-333333333333"
	require.NoError(t, userRepo.Create(ctx, &identity.User{
		ID: uid, Email: "s@example.com", PasswordHash: "h", Locale: "en", Timezone: "UTC", CreatedAt: time.Now().UTC(),
	}))

	s := &identity.Session{
		ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", UserID: uid, HydraSessionID: "sid-1",
		DeviceName: "Chrome on Mac", UserAgent: "UA", IP: "1.2.3.4",
		CreatedAt: time.Now().UTC(), LastSeenAt: time.Now().UTC(),
	}
	require.NoError(t, sessRepo.Create(ctx, s))

	active, err := sessRepo.ListActiveByUser(ctx, uid)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, "sid-1", active[0].HydraSessionID)

	require.NoError(t, sessRepo.MarkEnded(ctx, s.ID))
	active, err = sessRepo.ListActiveByUser(ctx, uid)
	require.NoError(t, err)
	require.Len(t, active, 0)

	_, err = sessRepo.FindByID(ctx, "no-such-id")
	require.ErrorIs(t, err, identity.ErrSessionNotFound)
}

func TestSessionRepositoryEndByHydraSIDAndAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	userRepo := NewUserRepository(w.pool)
	sessRepo := NewSessionRepository(w.pool)

	uid := "44444444-4444-4444-4444-444444444444"
	require.NoError(t, userRepo.Create(ctx, &identity.User{
		ID: uid, Email: "m@example.com", PasswordHash: "h", Locale: "en", Timezone: "UTC", CreatedAt: time.Now().UTC(),
	}))
	mk := func(id, sid string) *identity.Session {
		return &identity.Session{ID: id, UserID: uid, HydraSessionID: sid, CreatedAt: time.Now().UTC(), LastSeenAt: time.Now().UTC()}
	}
	require.NoError(t, sessRepo.Create(ctx, mk("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "sid-A")))
	require.NoError(t, sessRepo.Create(ctx, mk("cccccccc-cccc-cccc-cccc-cccccccccccc", "sid-B")))

	require.NoError(t, sessRepo.MarkEndedByHydraSID(ctx, "sid-A"))
	active, _ := sessRepo.ListActiveByUser(ctx, uid)
	require.Len(t, active, 1)

	require.NoError(t, sessRepo.MarkAllEndedByUser(ctx, uid))
	active, _ = sessRepo.ListActiveByUser(ctx, uid)
	require.Len(t, active, 0)
}
```
(NOTE: sessions FK-reference users, so each test seeds a user via `userRepo.Create` before creating sessions.)

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/postgres/ -run TestSessionRepository -v`
Expected: FAIL (нет `NewSessionRepository`).

- [ ] **Step 3: Реализовать** — `platform/internal/infrastructure/postgres/session_repository.go`:
```go
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/papyrus/platform/internal/domain/identity"
)

// SessionRepository is a pgx-backed identity.SessionRepository.
type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

func (r *SessionRepository) Create(ctx context.Context, s *identity.Session) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, hydra_session_id, device_name, user_agent, ip, location, created_at, last_seen_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		s.ID, s.UserID, s.HydraSessionID, s.DeviceName, s.UserAgent, s.IP, s.Location, s.CreatedAt, s.LastSeenAt)
	if err != nil {
		return fmt.Errorf("postgres: create session: %w", err)
	}
	return nil
}

func (r *SessionRepository) FindByID(ctx context.Context, id string) (*identity.Session, error) {
	var s identity.Session
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, hydra_session_id, device_name, user_agent, ip, location, created_at, last_seen_at, ended_at
		 FROM sessions WHERE id=$1`, id).
		Scan(&s.ID, &s.UserID, &s.HydraSessionID, &s.DeviceName, &s.UserAgent, &s.IP, &s.Location, &s.CreatedAt, &s.LastSeenAt, &s.EndedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, identity.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find session: %w", err)
	}
	return &s, nil
}

func (r *SessionRepository) ListActiveByUser(ctx context.Context, userID string) ([]*identity.Session, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, hydra_session_id, device_name, user_agent, ip, location, created_at, last_seen_at, ended_at
		 FROM sessions WHERE user_id=$1 AND ended_at IS NULL ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list sessions: %w", err)
	}
	defer rows.Close()

	var out []*identity.Session
	for rows.Next() {
		var s identity.Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.HydraSessionID, &s.DeviceName, &s.UserAgent, &s.IP, &s.Location, &s.CreatedAt, &s.LastSeenAt, &s.EndedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan session: %w", err)
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

func (r *SessionRepository) MarkEnded(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET ended_at=now() WHERE id=$1 AND ended_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("postgres: end session: %w", err)
	}
	return nil
}

func (r *SessionRepository) MarkEndedByHydraSID(ctx context.Context, sid string) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET ended_at=now() WHERE hydra_session_id=$1 AND ended_at IS NULL`, sid)
	if err != nil {
		return fmt.Errorf("postgres: end session by sid: %w", err)
	}
	return nil
}

func (r *SessionRepository) MarkAllEndedByUser(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET ended_at=now() WHERE user_id=$1 AND ended_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("postgres: end all sessions: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/postgres/ -run TestSessionRepository -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/infrastructure/postgres/
git commit -m "feat(platform): postgres session repository"
```

---

## Task 4: Use-case Authenticate (проверка email+пароль)

**Files:**
- Create: `platform/internal/application/identity/authenticate.go`
- Test: `platform/internal/application/identity/authenticate_test.go`

- [ ] **Step 1: Написать падающий тест** — `platform/internal/application/identity/authenticate_test.go`:
```go
package identity_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
)

func TestAuthenticateSuccess(t *testing.T) {
	users := newFakeUsers()
	_ = users.Create(context.Background(), &domain.User{
		ID: "u1", Email: "a@x.com", PasswordHash: "hashed:pw", EmailVerified: true,
	})
	uc := identity.NewAuthenticate(users, fakeHasher{})

	u, err := uc.Execute(context.Background(), "A@x.com", "pw") // email case-insensitive
	require.NoError(t, err)
	require.Equal(t, "u1", u.ID)
}

func TestAuthenticateWrongPassword(t *testing.T) {
	users := newFakeUsers()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "hashed:pw", EmailVerified: true})
	uc := identity.NewAuthenticate(users, fakeHasher{})
	_, err := uc.Execute(context.Background(), "a@x.com", "wrong")
	require.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestAuthenticateUnknownUserIsInvalidCredentials(t *testing.T) {
	uc := identity.NewAuthenticate(newFakeUsers(), fakeHasher{})
	_, err := uc.Execute(context.Background(), "ghost@x.com", "pw")
	require.ErrorIs(t, err, domain.ErrInvalidCredentials) // NOT ErrUserNotFound (no enumeration)
}

func TestAuthenticateUnverifiedEmail(t *testing.T) {
	users := newFakeUsers()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "hashed:pw", EmailVerified: false})
	uc := identity.NewAuthenticate(users, fakeHasher{})
	_, err := uc.Execute(context.Background(), "a@x.com", "pw")
	require.ErrorIs(t, err, domain.ErrEmailNotVerified)
}
```
(Reuses `fakeUsers`/`fakeHasher` from `fakes_test.go` created in Phase 2a Task 7.)

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/application/identity/ -run TestAuthenticate -v`
Expected: FAIL (нет `NewAuthenticate`).

- [ ] **Step 3: Реализовать** — `platform/internal/application/identity/authenticate.go`:
```go
package identity

import (
	"context"
	"errors"
	"strings"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

// Authenticate verifies an email/password pair and returns the user.
type Authenticate struct {
	users  domain.UserRepository
	hasher domain.PasswordHasher
}

func NewAuthenticate(users domain.UserRepository, hasher domain.PasswordHasher) *Authenticate {
	return &Authenticate{users: users, hasher: hasher}
}

func (uc *Authenticate) Execute(ctx context.Context, email, password string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := uc.users.FindByEmail(ctx, email)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil, domain.ErrInvalidCredentials // do not reveal which part failed
	}
	if err != nil {
		return nil, err
	}
	if !uc.hasher.Check(u.PasswordHash, password) {
		return nil, domain.ErrInvalidCredentials
	}
	if !u.EmailVerified {
		return nil, domain.ErrEmailNotVerified
	}
	return u, nil
}
```

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/application/identity/ -run TestAuthenticate -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/application/identity/
git commit -m "feat(platform): Authenticate use-case"
```

---

## Task 5: HTML-шаблоны login + рендерер

**Files:**
- Create: `platform/internal/presentation/http/templates/login.html`
- Create: `platform/internal/presentation/http/templates.go`
- Test: `platform/internal/presentation/http/templates_test.go`

- [ ] **Step 1: Написать шаблон** — `platform/internal/presentation/http/templates/login.html`:
```html
<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>Sign in — Papyrus</title></head>
<body>
  <h1>Sign in</h1>
  {{if .Error}}<p role="alert">{{.Error}}</p>{{end}}
  <form method="post" action="/login">
    <input type="hidden" name="login_challenge" value="{{.Challenge}}">
    <label>Email <input type="email" name="email" required></label>
    <label>Password <input type="password" name="password" required></label>
    <button type="submit">Sign in</button>
  </form>
</body>
</html>
```

- [ ] **Step 2: Написать падающий тест рендерера** — `platform/internal/presentation/http/templates_test.go`:
```go
package http

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderLogin(t *testing.T) {
	tpl := MustLoadTemplates()
	var buf bytes.Buffer
	require.NoError(t, tpl.ExecuteTemplate(&buf, "login.html", map[string]any{
		"Challenge": "chal-123", "Error": "",
	}))
	out := buf.String()
	require.True(t, strings.Contains(out, `value="chal-123"`))
	require.True(t, strings.Contains(out, `action="/login"`))
}
```

- [ ] **Step 3: Запустить — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/presentation/http/ -run TestRenderLogin -v`
Expected: FAIL (нет `MustLoadTemplates`).

- [ ] **Step 4: Реализовать рендерер** — `platform/internal/presentation/http/templates.go`:
```go
package http

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templatesFS embed.FS

// MustLoadTemplates parses all embedded HTML templates or panics.
func MustLoadTemplates() *template.Template {
	return template.Must(template.ParseFS(templatesFS, "templates/*.html"))
}
```

- [ ] **Step 5: Запустить — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/presentation/http/ -run TestRenderLogin -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/presentation/http/templates.go platform/internal/presentation/http/templates_test.go platform/internal/presentation/http/templates/
git commit -m "feat(platform): html template loader + login template"
```

---

## Task 6: Login + Consent хендлеры (TDD на фейках)

**Files:**
- Create: `platform/internal/presentation/http/auth_handlers.go`
- Test: `platform/internal/presentation/http/auth_handlers_test.go`
- Modify: `platform/internal/presentation/http/fakes_test.go` (добавить fakeHydra + fakeSessions)

**Логика:**
- `GET /login?login_challenge=X`: `GetLoginRequest`. Если `Skip` → `AcceptLoginRequest(subject)` → 302 на redirectTo. Иначе рендер `login.html` с challenge.
- `POST /login` (form: login_challenge, email, password): `Authenticate`. Успех → `AcceptLoginRequest(challenge, subject=userID, remember=true)` → 302 на redirectTo. Ошибка креды/верификации → 200 рендер `login.html` с Error.
- `GET /consent?consent_challenge=X`: `GetConsentRequest`. Для trusted-клиента → `AcceptConsentRequest(grantScopes=RequestedScopes)` → создать `Session` (id=uuid, UserID=Subject, HydraSessionID=LoginSessionID, UA/IP из запроса, device из UA) → 302 на redirectTo.

- [ ] **Step 1: Добавить фейки** — в `platform/internal/presentation/http/fakes_test.go` добавить:
```go
import "github.com/papyrus/platform/internal/domain/identity"

// fakeHydra implements identity.HydraClient.
type fakeHydra struct {
	login       *identity.HydraLoginRequest
	consent     *identity.HydraConsentRequest
	acceptedSub string
	grantedScopes []string
	redirect    string
}

func (f *fakeHydra) GetLoginRequest(_ context.Context, ch string) (*identity.HydraLoginRequest, error) {
	if f.login == nil {
		return &identity.HydraLoginRequest{Challenge: ch}, nil
	}
	f.login.Challenge = ch
	return f.login, nil
}
func (f *fakeHydra) AcceptLoginRequest(_ context.Context, ch, sub string, _ bool) (string, error) {
	f.acceptedSub = sub
	return f.redirect, nil
}
func (f *fakeHydra) RejectLoginRequest(_ context.Context, ch, reason string) (string, error) {
	return f.redirect, nil
}
func (f *fakeHydra) GetConsentRequest(_ context.Context, ch string) (*identity.HydraConsentRequest, error) {
	if f.consent == nil {
		return &identity.HydraConsentRequest{Challenge: ch, Client: identity.OAuthClientInfo{Trusted: true}}, nil
	}
	f.consent.Challenge = ch
	return f.consent, nil
}
func (f *fakeHydra) AcceptConsentRequest(_ context.Context, ch string, scopes []string) (string, error) {
	f.grantedScopes = scopes
	return f.redirect, nil
}

// fakeSessions implements identity.SessionRepository (create-capturing only for these tests).
type fakeSessions struct{ created []*identity.Session }

func (f *fakeSessions) Create(_ context.Context, s *identity.Session) error {
	f.created = append(f.created, s)
	return nil
}
func (f *fakeSessions) FindByID(_ context.Context, id string) (*identity.Session, error) {
	return nil, identity.ErrSessionNotFound
}
func (f *fakeSessions) ListActiveByUser(_ context.Context, _ string) ([]*identity.Session, error) {
	return f.created, nil
}
func (f *fakeSessions) MarkEnded(_ context.Context, _ string) error            { return nil }
func (f *fakeSessions) MarkEndedByHydraSID(_ context.Context, _ string) error  { return nil }
func (f *fakeSessions) MarkAllEndedByUser(_ context.Context, _ string) error   { return nil }
```

- [ ] **Step 2: Написать падающий тест** — `platform/internal/presentation/http/auth_handlers_test.go`:
```go
package http_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	appidentity "github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

func buildAuthServer(t *testing.T) (*httptest.Server, *fakeHydra, *fakeSessions, *fakeUsers) {
	t.Helper()
	users := newFakeUsers()
	sessions := &fakeSessions{}
	hydra := &fakeHydra{redirect: "https://hydra/redirect"}
	h := apphttp.NewAuthHandlers(
		appidentity.NewAuthenticate(users, fakeHasher{}),
		hydra, sessions, apphttp.MustLoadTemplates(),
	)
	r := chi.NewRouter()
	h.Register(r)
	// client that does NOT auto-follow redirects
	srv := httptest.NewServer(r)
	return srv, hydra, sessions, users
}

func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func TestLoginGETRendersForm(t *testing.T) {
	srv, _, _, _ := buildAuthServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/login?login_challenge=chal-1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	require.Contains(t, string(body[:n]), `value="chal-1"`)
}

func TestLoginPOSTSuccessAcceptsAndRedirects(t *testing.T) {
	srv, hydra, _, users := buildAuthServer(t)
	defer srv.Close()
	_ = users.Create(nil, &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "hashed:pw", EmailVerified: true})

	form := url.Values{"login_challenge": {"chal-1"}, "email": {"a@x.com"}, "password": {"pw"}}
	resp, err := noRedirectClient().PostForm(srv.URL+"/login", form)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode) // 302
	require.Equal(t, "https://hydra/redirect", resp.Header.Get("Location"))
	require.Equal(t, "u1", hydra.acceptedSub)
}

func TestLoginPOSTBadPasswordRerendersForm(t *testing.T) {
	srv, _, _, users := buildAuthServer(t)
	defer srv.Close()
	_ = users.Create(nil, &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "hashed:pw", EmailVerified: true})

	form := url.Values{"login_challenge": {"chal-1"}, "email": {"a@x.com"}, "password": {"WRONG"}}
	resp, err := noRedirectClient().PostForm(srv.URL+"/login", form)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode) // re-render, not redirect
	body := make([]byte, 8192)
	n, _ := resp.Body.Read(body)
	require.True(t, strings.Contains(string(body[:n]), "form")) // form shown again
}

func TestConsentAutoAcceptsTrustedAndCreatesSession(t *testing.T) {
	srv, hydra, sessions, _ := buildAuthServer(t)
	defer srv.Close()
	hydra.consent = &domain.HydraConsentRequest{
		Subject: "u1", LoginSessionID: "sid-xyz",
		RequestedScopes: []string{"openid", "profile"},
		Client:          domain.OAuthClientInfo{ID: "papyrus", Trusted: true},
	}

	resp, err := noRedirectClient().Get(srv.URL + "/consent?consent_challenge=cc-1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, []string{"openid", "profile"}, hydra.grantedScopes)
	require.Len(t, sessions.created, 1)
	require.Equal(t, "u1", sessions.created[0].UserID)
	require.Equal(t, "sid-xyz", sessions.created[0].HydraSessionID)
}
```

- [ ] **Step 3: Запустить — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/presentation/http/ -run "TestLogin|TestConsent" -v`
Expected: FAIL (нет `NewAuthHandlers`).

- [ ] **Step 4: Реализовать** — `platform/internal/presentation/http/auth_handlers.go`:
```go
package http

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	appidentity "github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
)

// AuthHandlers implements the Hydra login/consent provider endpoints.
type AuthHandlers struct {
	authenticate *appidentity.Authenticate
	hydra        domain.HydraClient
	sessions     domain.SessionRepository
	tpl          *template.Template
}

func NewAuthHandlers(authenticate *appidentity.Authenticate, hydra domain.HydraClient,
	sessions domain.SessionRepository, tpl *template.Template) *AuthHandlers {
	return &AuthHandlers{authenticate: authenticate, hydra: hydra, sessions: sessions, tpl: tpl}
}

func (h *AuthHandlers) Register(r chi.Router) {
	r.Get("/login", h.getLogin)
	r.Post("/login", h.postLogin)
	r.Get("/consent", h.getConsent)
}

func (h *AuthHandlers) renderLogin(w http.ResponseWriter, challenge, errMsg string, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = h.tpl.ExecuteTemplate(w, "login.html", map[string]any{"Challenge": challenge, "Error": errMsg})
}

func (h *AuthHandlers) getLogin(w http.ResponseWriter, r *http.Request) {
	challenge := r.URL.Query().Get("login_challenge")
	req, err := h.hydra.GetLoginRequest(r.Context(), challenge)
	if err != nil {
		http.Error(w, "login flow error", http.StatusBadGateway)
		return
	}
	if req.Skip {
		redirect, err := h.hydra.AcceptLoginRequest(r.Context(), challenge, req.Subject, true)
		if err != nil {
			http.Error(w, "login flow error", http.StatusBadGateway)
			return
		}
		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}
	h.renderLogin(w, challenge, "", http.StatusOK)
}

func (h *AuthHandlers) postLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	challenge := r.PostForm.Get("login_challenge")
	email := r.PostForm.Get("email")
	password := r.PostForm.Get("password")

	u, err := h.authenticate.Execute(r.Context(), email, password)
	if err != nil {
		msg := "Invalid email or password"
		if errors.Is(err, domain.ErrEmailNotVerified) {
			msg = "Please verify your email first"
		}
		h.renderLogin(w, challenge, msg, http.StatusOK)
		return
	}

	redirect, err := h.hydra.AcceptLoginRequest(r.Context(), challenge, u.ID, true)
	if err != nil {
		http.Error(w, "login flow error", http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (h *AuthHandlers) getConsent(w http.ResponseWriter, r *http.Request) {
	challenge := r.URL.Query().Get("consent_challenge")
	req, err := h.hydra.GetConsentRequest(r.Context(), challenge)
	if err != nil {
		http.Error(w, "consent flow error", http.StatusBadGateway)
		return
	}
	// MVP: our clients are trusted → auto-accept all requested scopes.
	redirect, err := h.hydra.AcceptConsentRequest(r.Context(), challenge, req.RequestedScopes)
	if err != nil {
		http.Error(w, "consent flow error", http.StatusBadGateway)
		return
	}
	// Record the session (sid available here as LoginSessionID).
	_ = h.sessions.Create(r.Context(), &domain.Session{
		ID:             uuid.NewString(),
		UserID:         req.Subject,
		HydraSessionID: req.LoginSessionID,
		DeviceName:     deviceFromUA(r.UserAgent()),
		UserAgent:      r.UserAgent(),
		IP:             clientIP(r),
	})
	http.Redirect(w, r, redirect, http.StatusFound)
}

func deviceFromUA(ua string) string {
	if ua == "" {
		return "Unknown device"
	}
	return ua // MVP: store raw UA; pretty parsing deferred
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}
```
(NOTE: `CreatedAt`/`LastSeenAt` are left zero here; the Postgres session table defaults them to `now()`. In the fake, they stay zero — fine for these unit tests. If you prefer explicit timestamps, set `time.Now().UTC()` — acceptable either way.)

- [ ] **Step 5: Запустить — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/presentation/http/ -v`
Expected: PASS (login GET/POST, consent).

- [ ] **Step 6: Commit** (`go mod tidy` if needed)

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/presentation/http/
git commit -m "feat(platform): hydra login/consent handlers"
```

---

## Task 7: Реальный HydraClient на Ory SDK

**Files:**
- Create: `platform/internal/infrastructure/hydra/client.go`

**API latitude:** the Ory SDK (`github.com/ory/hydra-client-go/v2`) surface varies by version. The code below is the intended shape; adapt method/field names to the ACTUAL installed SDK so it compiles. Acceptance for this task is: it compiles and implements `identity.HydraClient`; behavior is verified by the E2E Docker test in Task 9 (there is no unit test here — the SDK does real HTTP).

- [ ] **Step 1: Добавить зависимость**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go get github.com/ory/hydra-client-go/v2@latest`

- [ ] **Step 2: Реализовать** — `platform/internal/infrastructure/hydra/client.go`:
```go
package hydra

import (
	"context"
	"fmt"

	ory "github.com/ory/hydra-client-go/v2"

	"github.com/papyrus/platform/internal/domain/identity"
)

// Client implements identity.HydraClient using the Ory Hydra admin API.
type Client struct {
	api *ory.APIClient
	// trusted holds OAuth client IDs treated as first-party (auto-consent).
	trusted map[string]bool
}

// New builds a Hydra admin client pointed at adminURL (e.g. http://hydra:4445),
// treating the given client IDs as trusted.
func New(adminURL string, trustedClientIDs []string) *Client {
	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: adminURL}}
	trusted := make(map[string]bool, len(trustedClientIDs))
	for _, id := range trustedClientIDs {
		trusted[id] = true
	}
	return &Client{api: ory.NewAPIClient(cfg), trusted: trusted}
}

func (c *Client) clientInfo(cl *ory.OAuth2Client) identity.OAuthClientInfo {
	info := identity.OAuthClientInfo{}
	if cl != nil {
		if cl.ClientId != nil {
			info.ID = *cl.ClientId
		}
		if cl.ClientName != nil {
			info.Name = *cl.ClientName
		}
		info.Trusted = c.trusted[info.ID]
	}
	return info
}

func (c *Client) GetLoginRequest(ctx context.Context, challenge string) (*identity.HydraLoginRequest, error) {
	req, _, err := c.api.OAuth2API.GetOAuth2LoginRequest(ctx).LoginChallenge(challenge).Execute()
	if err != nil {
		return nil, fmt.Errorf("hydra: get login request: %w", err)
	}
	out := &identity.HydraLoginRequest{Challenge: challenge, Skip: req.Skip, Client: c.clientInfo(req.Client)}
	if req.Subject != "" {
		out.Subject = req.Subject
	}
	return out, nil
}

func (c *Client) AcceptLoginRequest(ctx context.Context, challenge, subject string, remember bool) (string, error) {
	body := ory.NewAcceptOAuth2LoginRequest(subject)
	body.SetRemember(remember)
	res, _, err := c.api.OAuth2API.AcceptOAuth2LoginRequest(ctx).
		LoginChallenge(challenge).AcceptOAuth2LoginRequest(*body).Execute()
	if err != nil {
		return "", fmt.Errorf("hydra: accept login: %w", err)
	}
	return res.RedirectTo, nil
}

func (c *Client) RejectLoginRequest(ctx context.Context, challenge, reason string) (string, error) {
	body := ory.NewRejectOAuth2Request()
	body.SetError(reason)
	res, _, err := c.api.OAuth2API.RejectOAuth2LoginRequest(ctx).
		LoginChallenge(challenge).RejectOAuth2Request(*body).Execute()
	if err != nil {
		return "", fmt.Errorf("hydra: reject login: %w", err)
	}
	return res.RedirectTo, nil
}

func (c *Client) GetConsentRequest(ctx context.Context, challenge string) (*identity.HydraConsentRequest, error) {
	req, _, err := c.api.OAuth2API.GetOAuth2ConsentRequest(ctx).ConsentChallenge(challenge).Execute()
	if err != nil {
		return nil, fmt.Errorf("hydra: get consent request: %w", err)
	}
	out := &identity.HydraConsentRequest{
		Challenge:       challenge,
		Skip:            req.Skip,
		RequestedScopes: req.RequestedScope,
		Client:          c.clientInfo(req.Client),
	}
	if req.Subject != nil {
		out.Subject = *req.Subject
	}
	if req.LoginSessionId != nil {
		out.LoginSessionID = *req.LoginSessionId
	}
	return out, nil
}

func (c *Client) AcceptConsentRequest(ctx context.Context, challenge string, grantScopes []string) (string, error) {
	body := ory.NewAcceptOAuth2ConsentRequest()
	body.SetGrantScope(grantScopes)
	res, _, err := c.api.OAuth2API.AcceptOAuth2ConsentRequest(ctx).
		ConsentChallenge(challenge).AcceptOAuth2ConsentRequest(*body).Execute()
	if err != nil {
		return "", fmt.Errorf("hydra: accept consent: %w", err)
	}
	return res.RedirectTo, nil
}
```

- [ ] **Step 3: Проверить компиляцию (адаптируя под реальный SDK)**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go build ./internal/infrastructure/hydra/`
Expected: компилируется. Если имена методов/полей SDK отличаются — открыть исходники SDK в module cache и поправить, сохраняя реализацию интерфейса `identity.HydraClient`.

- [ ] **Step 4: Убедиться, что реализует интерфейс** — добавить compile-time assertion в конец `client.go`:
```go
var _ identity.HydraClient = (*Client)(nil)
```
Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go build ./...`
Expected: чисто.

- [ ] **Step 5: Commit** (`go mod tidy`)

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go mod tidy
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/infrastructure/hydra/ platform/go.mod platform/go.sum
git commit -m "feat(platform): ory hydra admin client"
```

---

## Task 8: DI-проводка + mount auth-роутов + trusted-клиенты в конфиге

**Files:**
- Modify: `platform/internal/config/config.go` (+ test) — `TrustedClientIDs`
- Modify: `platform/internal/infrastructure/di/wire.go` (+ regen)
- Modify: `platform/internal/infrastructure/httpserver/server.go` — mount auth routes
- Modify: `platform/internal/presentation/http/identity_handlers.go` НЕ трогаем; auth routes монтируются отдельно

- [ ] **Step 1: Конфиг — добавить TrustedClientIDs (тест)**

Add to `platform/internal/config/config_test.go`:
```go
func TestLoadReadsTrustedClients(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_NAME", "n")
	t.Setenv("REDIS_HOST", "r")
	t.Setenv("REDIS_PORT", "6379")
	t.Setenv("TRUSTED_CLIENT_IDS", "papyrus,lite")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, []string{"papyrus", "lite"}, cfg.TrustedClientIDs)
}
```

- [ ] **Step 2: Запустить — падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/config/ -run TestLoadReadsTrustedClients -v`
Expected: FAIL.

- [ ] **Step 3: Реализовать в config.go** — добавить поле `TrustedClientIDs []string` в `Config` и в `Load()` (перед `return`):
```go
	if raw := os.Getenv("TRUSTED_CLIENT_IDS"); raw != "" {
		for _, id := range strings.Split(raw, ",") {
			if s := strings.TrimSpace(id); s != "" {
				cfg.TrustedClientIDs = append(cfg.TrustedClientIDs, s)
			}
		}
	}
```
(добавить `import "strings"` если ещё не импортирован.)

- [ ] **Step 4: Запустить — проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Обновить wire.go** — добавить провайдеры и смонтировать auth-хендлеры. Добавить в `provideServer` зависимость от `*apphttp.AuthHandlers`, и новые провайдеры:
```go
func provideSessionRepo(pool *pgxpool.Pool) domainidentity.SessionRepository {
	return pgc.NewSessionRepository(pool)
}

func provideHydraClient(cfg config.Config) domainidentity.HydraClient {
	return hydra.New(cfg.Hydra.AdminURL, cfg.TrustedClientIDs)
}

func provideAuthHandlers(users domainidentity.UserRepository, hasher domainidentity.PasswordHasher,
	hydraClient domainidentity.HydraClient, sessions domainidentity.SessionRepository) *apphttp.AuthHandlers {
	return apphttp.NewAuthHandlers(
		appidentity.NewAuthenticate(users, hasher),
		hydraClient, sessions, apphttp.MustLoadTemplates(),
	)
}
```
Обновить `provideServer`:
```go
func provideServer(cfg config.Config, identity *apphttp.IdentityHandlers, auth *apphttp.AuthHandlers) *http.Server {
	return httpserver.NewServer(":"+cfg.Port, httpserver.NewRouter(identity, auth))
}
```
Добавить в `wire.Build(...)`: `provideSessionRepo, provideHydraClient, provideAuthHandlers` и импорт пакета `hydra`.

- [ ] **Step 6: Обновить NewRouter** — `platform/internal/infrastructure/httpserver/server.go`:
```go
func NewRouter(identity *apphttp.IdentityHandlers, auth *apphttp.AuthHandlers) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", apphttp.Healthz())
	identity.Register(r)
	auth.Register(r)

	return r
}
```
И обновить `server_test.go`: `NewRouter(apphttp.NewIdentityHandlers(nil,nil,nil,nil), apphttp.NewAuthHandlers(nil,nil,nil,apphttp.MustLoadTemplates()))` — только `/healthz` тестируется, nil-зависимости не вызываются. (Templates нужны ненулевые, т.к. AuthHandlers их хранит; MustLoadTemplates безопасен.)

- [ ] **Step 7: Regen wire + build**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && make wire && go build ./... && go test -short ./...`
Expected: wire regen ок, build чисто, юнит-тесты зелёные.

- [ ] **Step 8: Обновить env** — добавить в `platform/.env.example` и в `docker-compose.yml` (env сервиса platform-core):
```
TRUSTED_CLIENT_IDS=papyrus
```

- [ ] **Step 9: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/config/ platform/internal/infrastructure/di/ platform/internal/infrastructure/httpserver/ platform/.env.example platform/docker-compose.yml
git commit -m "feat(platform): wire hydra login/consent + trusted clients"
```

---

## Task 9: Bootstrap trusted-клиента + E2E OAuth-флоу через Docker

**Files:**
- Create: `platform/cmd/bootstrap-client/main.go`

- [ ] **Step 1: Написать one-shot регистратор клиента** — `platform/cmd/bootstrap-client/main.go`:

Registers (idempotently) an OAuth2 client `papyrus` in Hydra with `authorization_code` grant, redirect URI, and scopes `openid profile`, using the Hydra admin API (Ory SDK). It reads `HYDRA_ADMIN_URL` and a redirect URI from env (`CLIENT_REDIRECT_URI`, default `http://localhost:5555/callback`). Implement with the same Ory SDK; adapt method names to the actual SDK (`CreateOAuth2Client`). On "client already exists" (409), treat as success. Print the client_id/secret.

Guidance (adapt to SDK):
```go
package main

import (
	"context"
	"log"
	"os"

	ory "github.com/ory/hydra-client-go/v2"
)

func main() {
	adminURL := os.Getenv("HYDRA_ADMIN_URL")
	if adminURL == "" {
		adminURL = "http://localhost:4445"
	}
	redirect := os.Getenv("CLIENT_REDIRECT_URI")
	if redirect == "" {
		redirect = "http://localhost:5555/callback"
	}
	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: adminURL}}
	api := ory.NewAPIClient(cfg)

	c := ory.NewOAuth2Client()
	c.SetClientId("papyrus")
	c.SetClientName("Papyrus")
	c.SetGrantTypes([]string{"authorization_code", "refresh_token"})
	c.SetResponseTypes([]string{"code"})
	c.SetRedirectUris([]string{redirect})
	c.SetScope("openid profile")
	c.SetTokenEndpointAuthMethod("none") // public client for the smoke test

	created, resp, err := api.OAuth2API.CreateOAuth2Client(context.Background()).OAuth2Client(*c).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == 409 {
			log.Println("client already exists — ok")
			return
		}
		log.Fatalf("create client: %v", err)
	}
	log.Printf("created client: %s", *created.ClientId)
}
```

- [ ] **Step 2: Проверить сборку**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go build ./cmd/bootstrap-client`
Expected: чисто (адаптировать под SDK при необходимости; не коммитить бинарь).

- [ ] **Step 3: E2E — поднять стек, зарегистрировать клиента, прогнать authorization_code флоу**

Run (single sequence):
```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && docker compose up -d --build --wait
```
Зарегистрировать клиента (через admin-порт Hydra, проброшен на 4445):
```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && HYDRA_ADMIN_URL=http://localhost:4445 go run ./cmd/bootstrap-client
```
Создать верифицированного пользователя и подтвердить (через наш API + прямое подтверждение). Так как email-верификация нужна для входа, проще всего в smoke пометить пользователя verified напрямую в БД:
```bash
curl -sf -X POST http://localhost:8090/register -H 'Content-Type: application/json' \
  -d '{"email":"login@example.com","password":"long-enough-pw","name":"Log"}'
docker exec platform-postgres psql -U platform -d platform -c \
  "UPDATE users SET email_verified=true WHERE email='login@example.com';"
```
Прогнать authorization_code флоу с эмуляцией браузера (cookie jar, следуем редиректам, POST формы логина). Используй `curl -c/-b` cookie-jar:
```bash
# 1. начинаем OAuth: /authorize (public port 4444) с cookie-jar, следуем редиректам до нашей формы логина
curl -sS -c /tmp/cj.txt -b /tmp/cj.txt -L \
  "http://localhost:4444/oauth2/auth?client_id=papyrus&response_type=code&scope=openid+profile&redirect_uri=http://localhost:5555/callback&state=xyz" \
  -o /tmp/login_page.html -w '%{url_effective}\n'
# ожидаем: url_effective — наш /login?login_challenge=...; в /tmp/login_page.html есть форма
```
Извлечь `login_challenge` из URL/страницы, отправить логин, дойти до консента (авто), получить `code` на redirect_uri. Точную последовательность curl-шагов реализуй и **проверь, что в итоге на redirect_uri прилетает `?code=...&state=xyz`**, а в БД появилась строка в `sessions` с непустым `hydra_session_id`:
```bash
docker exec platform-postgres psql -U platform -d platform -c \
  "SELECT user_id, hydra_session_id, user_agent FROM sessions;"
```
Expected: authorization code получен; в `sessions` есть запись с `hydra_session_id`.

**Acceptance:** полный OAuth authorization_code флоу проходит через наш login (ввод email/пароля) + авто-consent, Hydra выдаёт `code`, и создаётся строка сессии с Hydra sid. Если какой-то шаг curl нестабилен, допустимо реализовать маленький Go-хелпер в `cmd/` для прогонки флоу — но НЕ коммить его без нужды; главное — доказать, что флоу работает, и описать результат.

- [ ] **Step 4: Погасить стек**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && docker compose down`

- [ ] **Step 5: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/cmd/bootstrap-client/
git commit -m "feat(platform): oauth client bootstrap + verify login flow e2e"
```

---

## Task 10: Финальная проверка фазы

- [ ] **Step 1: Юнит-тесты**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test -short ./...`
Expected: PASS.

- [ ] **Step 2: Полные тесты (testcontainers)**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./...`
Expected: PASS (включая session repository, migrate sessions).

- [ ] **Step 3: vet + build**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go vet ./... && go build ./...`
Expected: чисто.

---

## Definition of Done (Фаза 2b-i)
- Таблица `sessions` создаётся миграцией (FK на users, индексы).
- `HydraClient` порт + реальная реализация на Ory SDK; хендлеры login/consent покрыты юнит-тестами на фейке Hydra.
- `GET /login` рендерит форму (или пропускает при Skip); `POST /login` проверяет креды (Authenticate: неверные → ErrInvalidCredentials, неверифицированный email → ErrEmailNotVerified) и принимает login в Hydra; `GET /consent` авто-принимает trusted-клиента и создаёт строку `sessions` с Hydra `sid` + устройство/IP/UA.
- E2E: полный authorization_code флоу проходит через наш login + авто-consent, Hydra выдаёт code, сессия записана.
- Все тесты зелёные; vet/build чистые.

## Следующая фаза
Фаза 2b-ii: завершение сессий (terminate-one по sid, logout-everywhere по subject через admin revoke) + logout-эндпоинт. Затем Фаза 2c: аккаунт-хаб UI (профиль, свитчер воркспейсов, список/управление сессиями, register/reset HTML).
