# Platform Core — Фаза 2a (Identity core) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Реализовать backend-ядро идентичности: таблица `users`, регистрация, верификация email, сброс пароля — с bcrypt-хешированием, Redis-токенами и Mailer-интерфейсом, доступное через JSON HTTP API.

**Architecture:** Продолжение Go-сервиса `platform/` (чистая архитектура). Порты (интерфейсы) в `domain/identity`, use-cases в `application/identity`, реализации в `infrastructure/*`, JSON-хендлеры в `presentation/http`. Пароли — bcrypt. Одноразовые токены (verify/reset) — в Redis с TTL. Email — порт `Mailer` с dev-реализацией `LogMailer` (печатает ссылку) и prod `SMTPMailer`. Миграции — golang-migrate со встроенными (`embed`) SQL, прогон при старте.

**Scope note:** 2a строит ТОЛЬКО backend + JSON API. Server-rendered HTML-формы (login/register/consent/reset) строятся в Фазе 2b вместе с экранами Hydra login/consent — чтобы html/template-слой не делать дважды. Hydra в 2a не трогаем.

**Tech Stack:** Go 1.26, chi, pgx/v5, go-redis/v9, google/uuid, golang.org/x/crypto/bcrypt, golang-migrate/v4 (iofs+pgx), testify, testcontainers.

---

## File Structure (эта фаза)

```
platform/internal/
  domain/identity/
    user.go               сущность User
    ports.go              UserRepository, PasswordHasher, Mailer, VerificationTokens
    errors.go             доменные ошибки
  application/identity/
    register_user.go      + register_user_test.go
    verify_email.go       + verify_email_test.go
    reset_password.go     + reset_password_test.go  (RequestPasswordReset + ResetPassword)
  infrastructure/
    security/password.go          + password_test.go   (bcrypt)
    postgres/
      migrations/0001_users.up.sql
      migrations/0001_users.down.sql
      migrate.go                  + migrate_test.go
      user_repository.go          + user_repository_test.go
    redis/token_store.go          + token_store_test.go
    mail/log_mailer.go            + log_mailer_test.go   (Mailer port lives in domain/identity)
    mail/smtp_mailer.go
  presentation/http/
    identity_handlers.go          + identity_handlers_test.go
```

Modified from Phase 1:
- `internal/infrastructure/httpserver/server.go` — `NewRouter` теперь принимает `*apphttp.IdentityHandlers`.
- `internal/infrastructure/di/wire.go` (+ regenerated `wire_gen.go`) — провайдеры новых зависимостей.
- `cmd/server/main.go` — прогон миграций перед стартом.

---

## Task 1: Доменная сущность User и порты

**Files:**
- Create: `platform/internal/domain/identity/user.go`
- Create: `platform/internal/domain/identity/ports.go`
- Create: `platform/internal/domain/identity/errors.go`

- [ ] **Step 1: Написать сущность** — `platform/internal/domain/identity/user.go`:
```go
package identity

import "time"

// User is a platform account (identity), independent of any workspace/product.
type User struct {
	ID            string
	Email         string
	EmailVerified bool
	PasswordHash  string
	Name          string
	AvatarURL     string
	Locale        string
	Timezone      string
	CreatedAt     time.Time
}
```

- [ ] **Step 2: Написать доменные ошибки** — `platform/internal/domain/identity/errors.go`:
```go
package identity

import "errors"

var (
	// ErrUserNotFound is returned by UserRepository lookups when no row matches.
	ErrUserNotFound = errors.New("identity: user not found")
	// ErrUserExists is returned when registering an email that already exists.
	ErrUserExists = errors.New("identity: user already exists")
	// ErrTokenInvalid is returned when a one-time token is missing or expired.
	ErrTokenInvalid = errors.New("identity: token invalid or expired")
	// ErrWeakPassword is returned when a password does not meet policy.
	ErrWeakPassword = errors.New("identity: password too weak")
	// ErrInvalidEmail is returned when an email is empty/malformed.
	ErrInvalidEmail = errors.New("identity: invalid email")
)
```

- [ ] **Step 3: Написать порты** — `platform/internal/domain/identity/ports.go`:
```go
package identity

import (
	"context"
	"time"
)

// Token purposes for one-time tokens.
const (
	PurposeVerifyEmail   = "verify_email"
	PurposePasswordReset = "password_reset"
)

// UserRepository persists users.
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	FindByEmail(ctx context.Context, email string) (*User, error) // ErrUserNotFound if absent
	FindByID(ctx context.Context, id string) (*User, error)       // ErrUserNotFound if absent
	Update(ctx context.Context, u *User) error
}

// PasswordHasher hashes and verifies passwords.
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Check(hash, plain string) bool
}

// Mailer sends transactional emails.
type Mailer interface {
	SendVerification(ctx context.Context, to, link string) error
	SendPasswordReset(ctx context.Context, to, link string) error
}

// VerificationTokens issues and consumes one-time tokens (backed by Redis + TTL).
type VerificationTokens interface {
	// Issue generates a random token bound to userID under purpose with ttl, returns the token string.
	Issue(ctx context.Context, purpose, userID string, ttl time.Duration) (string, error)
	// Consume validates and deletes the token, returning the bound userID, or ErrTokenInvalid.
	Consume(ctx context.Context, purpose, token string) (string, error)
}
```

- [ ] **Step 4: Проверить компиляцию**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go build ./internal/domain/...`
Expected: без ошибок (интерфейсы/типы компилируются).

- [ ] **Step 5: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/domain/identity/
git commit -m "feat(platform): identity domain (User, ports, errors)"
```

---

## Task 2: bcrypt PasswordHasher

**Files:**
- Create: `platform/internal/infrastructure/security/password.go`
- Test: `platform/internal/infrastructure/security/password_test.go`

- [ ] **Step 1: Написать падающий тест** — `platform/internal/infrastructure/security/password_test.go`:
```go
package security

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashAndCheck(t *testing.T) {
	h := NewBcryptHasher(0) // 0 → default cost

	hash, err := h.Hash("s3cret-password")
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	require.NotEqual(t, "s3cret-password", hash)

	require.True(t, h.Check(hash, "s3cret-password"))
	require.False(t, h.Check(hash, "wrong-password"))
}
```

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/security/ -v`
Expected: FAIL (нет `NewBcryptHasher`).

- [ ] **Step 3: Реализовать** — `platform/internal/infrastructure/security/password.go`:
```go
package security

import "golang.org/x/crypto/bcrypt"

// BcryptHasher implements identity.PasswordHasher using bcrypt.
type BcryptHasher struct {
	cost int
}

// NewBcryptHasher returns a hasher. cost<=0 uses bcrypt.DefaultCost.
func NewBcryptHasher(cost int) *BcryptHasher {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	return &BcryptHasher{cost: cost}
}

func (h *BcryptHasher) Hash(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (h *BcryptHasher) Check(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
```

- [ ] **Step 4: Запустить тест — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/security/ -v`
Expected: PASS.

- [ ] **Step 5: Commit** (run `go mod tidy` if x/crypto becomes direct)

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/infrastructure/security/ platform/go.mod platform/go.sum
git commit -m "feat(platform): bcrypt password hasher"
```

---

## Task 3: Миграция users + runner (golang-migrate, embed)

**Files:**
- Create: `platform/internal/infrastructure/postgres/migrations/0001_users.up.sql`
- Create: `platform/internal/infrastructure/postgres/migrations/0001_users.down.sql`
- Create: `platform/internal/infrastructure/postgres/migrate.go`
- Test: `platform/internal/infrastructure/postgres/migrate_test.go`

**API latitude:** golang-migrate v4 wiring has version-specific details (driver scheme, source instance). The implementation below is the intended approach; if the exact golang-migrate v4 API differs, adapt it so that (a) migrations are embedded via `embed.FS` (the final Docker image has only the binary — reading SQL from disk is NOT acceptable), and (b) the test passes. Keep the exported function `RunMigrations(dsn string) error` and the migrations directory location.

- [ ] **Step 1: Написать SQL миграции**

`platform/internal/infrastructure/postgres/migrations/0001_users.up.sql`:
```sql
CREATE TABLE users (
    id             UUID PRIMARY KEY,
    email          TEXT NOT NULL UNIQUE,
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    password_hash  TEXT NOT NULL,
    name           TEXT NOT NULL DEFAULT '',
    avatar_url     TEXT NOT NULL DEFAULT '',
    locale         TEXT NOT NULL DEFAULT 'en',
    timezone       TEXT NOT NULL DEFAULT 'UTC',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`platform/internal/infrastructure/postgres/migrations/0001_users.down.sql`:
```sql
DROP TABLE users;
```

- [ ] **Step 2: Написать падающий тест** — `platform/internal/infrastructure/postgres/migrate_test.go`:
```go
package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestRunMigrationsCreatesUsers(t *testing.T) {
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

	// running again must be a no-op (idempotent)
	require.NoError(t, RunMigrations(dsn))

	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	var exists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'users')`).
		Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists)
}
```

- [ ] **Step 3: Запустить тест — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/postgres/ -run TestRunMigrations -v`
Expected: FAIL (нет `RunMigrations`).

- [ ] **Step 4: Добавить зависимость и реализовать** — сначала:
```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go get github.com/golang-migrate/migrate/v4@latest
```
`platform/internal/infrastructure/postgres/migrate.go`:
```go
package postgres

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" sql driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// RunMigrations applies all up migrations. Idempotent (no-op if already current).
func RunMigrations(dsn string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("postgres: migration source: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("postgres: open for migrate: %w", err)
	}
	defer func() { _ = db.Close() }()

	driver, err := migratepgx.WithInstance(db, &migratepgx.Config{})
	if err != nil {
		return fmt.Errorf("postgres: migrate driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "pgx", driver)
	if err != nil {
		return fmt.Errorf("postgres: migrate init: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("postgres: migrate up: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Запустить тест — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/postgres/ -run TestRunMigrations -v`
Expected: PASS (создаёт users, повторный вызов — no-op).

- [ ] **Step 6: Commit** (`go mod tidy` first)

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go mod tidy
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/infrastructure/postgres/ platform/go.mod platform/go.sum
git commit -m "feat(platform): users migration + golang-migrate runner"
```

---

## Task 4: Postgres UserRepository

**Files:**
- Create: `platform/internal/infrastructure/postgres/user_repository.go`
- Test: `platform/internal/infrastructure/postgres/user_repository_test.go`

- [ ] **Step 1: Написать падающий интеграционный тест** — `platform/internal/infrastructure/postgres/user_repository_test.go`:
```go
package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/papyrus/platform/internal/domain/identity"
)

func newMigratedPool(t *testing.T) (context.Context, *pgxpoolWrapper) {
	t.Helper()
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
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, RunMigrations(dsn))

	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return ctx, &pgxpoolWrapper{pool}
}

func TestUserRepositoryCreateAndFind(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	repo := NewUserRepository(w.pool)

	u := &identity.User{
		ID: "11111111-1111-1111-1111-111111111111", Email: "a@example.com",
		PasswordHash: "hash", Name: "Alice", Locale: "en", Timezone: "UTC",
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, u))

	byEmail, err := repo.FindByEmail(ctx, "a@example.com")
	require.NoError(t, err)
	require.Equal(t, u.ID, byEmail.ID)
	require.False(t, byEmail.EmailVerified)

	byID, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "a@example.com", byID.Email)

	_, err = repo.FindByEmail(ctx, "missing@example.com")
	require.ErrorIs(t, err, identity.ErrUserNotFound)
}

func TestUserRepositoryUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	repo := NewUserRepository(w.pool)

	u := &identity.User{
		ID: "22222222-2222-2222-2222-222222222222", Email: "b@example.com",
		PasswordHash: "h", Locale: "en", Timezone: "UTC", CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, u))

	u.EmailVerified = true
	u.PasswordHash = "newhash"
	require.NoError(t, repo.Update(ctx, u))

	got, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	require.True(t, got.EmailVerified)
	require.Equal(t, "newhash", got.PasswordHash)
}
```
(NOTE: `pgxpoolWrapper` is a tiny local test helper defined below so the helper can return the pool typed; the implementer may inline the pool instead — the key is the repo takes a `*pgxpool.Pool`.)

Add helper in the same test file:
```go
import "github.com/jackc/pgx/v5/pgxpool"

type pgxpoolWrapper struct{ pool *pgxpool.Pool }
```

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/postgres/ -run TestUserRepository -v`
Expected: FAIL (нет `NewUserRepository`).

- [ ] **Step 3: Реализовать** — `platform/internal/infrastructure/postgres/user_repository.go`:
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

// UserRepository is a pgx-backed identity.UserRepository.
type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, u *identity.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, email, email_verified, password_hash, name, avatar_url, locale, timezone, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		u.ID, u.Email, u.EmailVerified, u.PasswordHash, u.Name, u.AvatarURL, u.Locale, u.Timezone, u.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create user: %w", err)
	}
	return nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*identity.User, error) {
	return r.scanOne(ctx,
		`SELECT id, email, email_verified, password_hash, name, avatar_url, locale, timezone, created_at
		 FROM users WHERE email = $1`, email)
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*identity.User, error) {
	return r.scanOne(ctx,
		`SELECT id, email, email_verified, password_hash, name, avatar_url, locale, timezone, created_at
		 FROM users WHERE id = $1`, id)
}

func (r *UserRepository) Update(ctx context.Context, u *identity.User) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET email=$2, email_verified=$3, password_hash=$4, name=$5, avatar_url=$6, locale=$7, timezone=$8
		 WHERE id=$1`,
		u.ID, u.Email, u.EmailVerified, u.PasswordHash, u.Name, u.AvatarURL, u.Locale, u.Timezone)
	if err != nil {
		return fmt.Errorf("postgres: update user: %w", err)
	}
	return nil
}

func (r *UserRepository) scanOne(ctx context.Context, query string, arg any) (*identity.User, error) {
	var u identity.User
	err := r.pool.QueryRow(ctx, query, arg).Scan(
		&u.ID, &u.Email, &u.EmailVerified, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Locale, &u.Timezone, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, identity.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find user: %w", err)
	}
	return &u, nil
}
```

- [ ] **Step 4: Запустить тест — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/postgres/ -run TestUserRepository -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/infrastructure/postgres/
git commit -m "feat(platform): postgres user repository"
```

---

## Task 5: Redis VerificationTokens (одноразовые токены)

**Files:**
- Create: `platform/internal/infrastructure/redis/token_store.go`
- Test: `platform/internal/infrastructure/redis/token_store_test.go`

- [ ] **Step 1: Написать падающий интеграционный тест** — `platform/internal/infrastructure/redis/token_store_test.go`:
```go
package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/papyrus/platform/internal/domain/identity"
)

func TestTokenStoreIssueConsume(t *testing.T) {
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

	store := NewTokenStore(client)

	token, err := store.Issue(ctx, identity.PurposeVerifyEmail, "user-1", time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	userID, err := store.Consume(ctx, identity.PurposeVerifyEmail, token)
	require.NoError(t, err)
	require.Equal(t, "user-1", userID)

	// second consume must fail (single use)
	_, err = store.Consume(ctx, identity.PurposeVerifyEmail, token)
	require.ErrorIs(t, err, identity.ErrTokenInvalid)

	// wrong purpose must fail
	token2, err := store.Issue(ctx, identity.PurposePasswordReset, "user-2", time.Minute)
	require.NoError(t, err)
	_, err = store.Consume(ctx, identity.PurposeVerifyEmail, token2)
	require.ErrorIs(t, err, identity.ErrTokenInvalid)
}
```

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/redis/ -run TestTokenStore -v`
Expected: FAIL (нет `NewTokenStore`).

- [ ] **Step 3: Реализовать** — `platform/internal/infrastructure/redis/token_store.go`:
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

	"github.com/papyrus/platform/internal/domain/identity"
)

// TokenStore implements identity.VerificationTokens using Redis with TTL.
type TokenStore struct {
	client *goredis.Client
}

func NewTokenStore(client *goredis.Client) *TokenStore {
	return &TokenStore{client: client}
}

func key(purpose, token string) string {
	return fmt.Sprintf("token:%s:%s", purpose, token)
}

func (s *TokenStore) Issue(ctx context.Context, purpose, userID string, ttl time.Duration) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("redis: generate token: %w", err)
	}
	token := hex.EncodeToString(buf)
	if err := s.client.Set(ctx, key(purpose, token), userID, ttl).Err(); err != nil {
		return "", fmt.Errorf("redis: store token: %w", err)
	}
	return token, nil
}

func (s *TokenStore) Consume(ctx context.Context, purpose, token string) (string, error) {
	// GETDEL: atomic get + delete (single-use).
	userID, err := s.client.GetDel(ctx, key(purpose, token)).Result()
	if errors.Is(err, goredis.Nil) {
		return "", identity.ErrTokenInvalid
	}
	if err != nil {
		return "", fmt.Errorf("redis: consume token: %w", err)
	}
	return userID, nil
}
```
(NOTE: the go-redis import is aliased `goredis` here because this file lives in `package redis`. The existing `redis.go` from Phase 1 imports it as `redis` — that's fine, imports are per-file; keep this file's alias `goredis` to avoid confusion.)

- [ ] **Step 4: Запустить тест — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/redis/ -run TestTokenStore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/infrastructure/redis/
git commit -m "feat(platform): redis one-time token store"
```

---

## Task 6: Mailer (порт + LogMailer + SMTPMailer)

**Files:**
- Create: `platform/internal/infrastructure/mail/log_mailer.go`
- Create: `platform/internal/infrastructure/mail/smtp_mailer.go`
- Test: `platform/internal/infrastructure/mail/log_mailer_test.go`

- [ ] **Step 1: Написать падающий тест LogMailer** — `platform/internal/infrastructure/mail/log_mailer_test.go`:
```go
package mail

import (
	"bytes"
	"context"
	"log"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogMailerLogsLink(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	m := NewLogMailer(logger)

	require.NoError(t, m.SendVerification(context.Background(), "a@example.com", "https://x/verify?token=abc"))

	out := buf.String()
	require.Contains(t, out, "a@example.com")
	require.Contains(t, out, "https://x/verify?token=abc")
}
```

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/mail/ -v`
Expected: FAIL (нет `NewLogMailer`).

- [ ] **Step 3: Реализовать LogMailer** — `platform/internal/infrastructure/mail/log_mailer.go`:
```go
package mail

import (
	"context"
	"log"
)

// LogMailer is a dev Mailer that logs the email link instead of sending.
type LogMailer struct {
	logger *log.Logger
}

func NewLogMailer(logger *log.Logger) *LogMailer {
	return &LogMailer{logger: logger}
}

func (m *LogMailer) SendVerification(_ context.Context, to, link string) error {
	m.logger.Printf("[mail] verification to=%s link=%s", to, link)
	return nil
}

func (m *LogMailer) SendPasswordReset(_ context.Context, to, link string) error {
	m.logger.Printf("[mail] password-reset to=%s link=%s", to, link)
	return nil
}
```

- [ ] **Step 4: Реализовать SMTPMailer** — `platform/internal/infrastructure/mail/smtp_mailer.go`:
```go
package mail

import (
	"context"
	"fmt"
	"net/smtp"
)

// SMTPMailer sends real email via SMTP. Used in production.
type SMTPMailer struct {
	addr string // host:port
	auth smtp.Auth
	from string
}

func NewSMTPMailer(host, port, user, password, from string) *SMTPMailer {
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, password, host)
	}
	return &SMTPMailer{addr: host + ":" + port, auth: auth, from: from}
}

func (m *SMTPMailer) send(to, subject, body string) error {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", m.from, to, subject, body)
	if err := smtp.SendMail(m.addr, m.auth, m.from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("mail: smtp send: %w", err)
	}
	return nil
}

func (m *SMTPMailer) SendVerification(_ context.Context, to, link string) error {
	return m.send(to, "Verify your email", "Confirm your email: "+link)
}

func (m *SMTPMailer) SendPasswordReset(_ context.Context, to, link string) error {
	return m.send(to, "Reset your password", "Reset your password: "+link)
}
```

- [ ] **Step 5: Запустить тест — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/infrastructure/mail/ -v`
Expected: PASS. (SMTPMailer has no unit test — it's a thin net/smtp wrapper exercised in integration later.)

- [ ] **Step 6: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/infrastructure/mail/
git commit -m "feat(platform): mailer port impls (log + smtp)"
```

---

## Task 7: Use-case RegisterUser

**Files:**
- Create: `platform/internal/application/identity/register_user.go`
- Test: `platform/internal/application/identity/register_user_test.go`

- [ ] **Step 1: Написать падающий тест с фейками** — `platform/internal/application/identity/register_user_test.go`:
```go
package identity_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
)

func TestRegisterUserCreatesUnverifiedAndSendsMail(t *testing.T) {
	users := newFakeUsers()
	tokens := newFakeTokens()
	mailer := newFakeMailer()
	uc := identity.NewRegisterUser(users, &fakeHasher{}, tokens, mailer, "https://acc.example")

	u, err := uc.Execute(context.Background(), identity.RegisterInput{
		Email: "  Alice@Example.com ", Password: "long-enough-pw", Name: "Alice",
	})
	require.NoError(t, err)
	require.NotEmpty(t, u.ID)
	require.Equal(t, "alice@example.com", u.Email) // normalized
	require.False(t, u.EmailVerified)
	require.Equal(t, "hashed:long-enough-pw", u.PasswordHash)

	// user persisted
	stored, err := users.FindByEmail(context.Background(), "alice@example.com")
	require.NoError(t, err)
	require.Equal(t, u.ID, stored.ID)

	// verification mail sent with a link containing the issued token
	require.Len(t, mailer.verifications, 1)
	require.Equal(t, "alice@example.com", mailer.verifications[0].to)
	require.True(t, strings.Contains(mailer.verifications[0].link, tokens.lastToken))
}

func TestRegisterUserRejectsDuplicate(t *testing.T) {
	users := newFakeUsers()
	uc := identity.NewRegisterUser(users, &fakeHasher{}, newFakeTokens(), newFakeMailer(), "https://acc.example")
	_, err := uc.Execute(context.Background(), identity.RegisterInput{Email: "a@x.com", Password: "long-enough-pw"})
	require.NoError(t, err)
	_, err = uc.Execute(context.Background(), identity.RegisterInput{Email: "a@x.com", Password: "long-enough-pw"})
	require.ErrorIs(t, err, domain.ErrUserExists)
}

func TestRegisterUserRejectsWeakPassword(t *testing.T) {
	uc := identity.NewRegisterUser(newFakeUsers(), &fakeHasher{}, newFakeTokens(), newFakeMailer(), "https://acc.example")
	_, err := uc.Execute(context.Background(), identity.RegisterInput{Email: "a@x.com", Password: "short"})
	require.ErrorIs(t, err, domain.ErrWeakPassword)
}

func TestRegisterUserRejectsEmptyEmail(t *testing.T) {
	uc := identity.NewRegisterUser(newFakeUsers(), &fakeHasher{}, newFakeTokens(), newFakeMailer(), "https://acc.example")
	_, err := uc.Execute(context.Background(), identity.RegisterInput{Email: "  ", Password: "long-enough-pw"})
	require.ErrorIs(t, err, domain.ErrInvalidEmail)
}
```

- [ ] **Step 2: Создать общие фейки для тестов пакета** — `platform/internal/application/identity/fakes_test.go`:
```go
package identity_test

import (
	"context"
	"time"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

type fakeUsers struct{ byID, byEmail map[string]*domain.User }

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[string]*domain.User{}, byEmail: map[string]*domain.User{}}
}
func (f *fakeUsers) Create(_ context.Context, u *domain.User) error {
	cp := *u
	f.byID[u.ID] = &cp
	f.byEmail[u.Email] = &cp
	return nil
}
func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := f.byEmail[email]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrUserNotFound
}
func (f *fakeUsers) FindByID(_ context.Context, id string) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrUserNotFound
}
func (f *fakeUsers) Update(_ context.Context, u *domain.User) error {
	cp := *u
	f.byID[u.ID] = &cp
	f.byEmail[u.Email] = &cp
	return nil
}

type fakeHasher struct{}

func (fakeHasher) Hash(plain string) (string, error) { return "hashed:" + plain, nil }
func (fakeHasher) Check(hash, plain string) bool      { return hash == "hashed:"+plain }

type fakeTokens struct {
	lastToken string
	store     map[string]string // purpose+token -> userID
}

func newFakeTokens() *fakeTokens { return &fakeTokens{store: map[string]string{}} }
func (f *fakeTokens) Issue(_ context.Context, purpose, userID string, _ time.Duration) (string, error) {
	f.lastToken = "tok-" + userID
	f.store[purpose+":"+f.lastToken] = userID
	return f.lastToken, nil
}
func (f *fakeTokens) Consume(_ context.Context, purpose, token string) (string, error) {
	k := purpose + ":" + token
	if uid, ok := f.store[k]; ok {
		delete(f.store, k)
		return uid, nil
	}
	return "", domain.ErrTokenInvalid
}

type sentMail struct{ to, link string }
type fakeMailer struct {
	verifications []sentMail
	resets        []sentMail
}

func newFakeMailer() *fakeMailer { return &fakeMailer{} }
func (f *fakeMailer) SendVerification(_ context.Context, to, link string) error {
	f.verifications = append(f.verifications, sentMail{to, link})
	return nil
}
func (f *fakeMailer) SendPasswordReset(_ context.Context, to, link string) error {
	f.resets = append(f.resets, sentMail{to, link})
	return nil
}
```

- [ ] **Step 3: Запустить тест — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/application/identity/ -v`
Expected: FAIL (нет `NewRegisterUser`/`RegisterInput`).

- [ ] **Step 4: Реализовать** — `platform/internal/application/identity/register_user.go`:
```go
package identity

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

const verifyTokenTTL = 24 * time.Hour
const minPasswordLen = 8

// RegisterInput is the request to register a new account.
type RegisterInput struct {
	Email    string
	Password string
	Name     string
	Locale   string
	Timezone string
}

// RegisterUser creates an unverified account and emails a verification link.
type RegisterUser struct {
	users   domain.UserRepository
	hasher  domain.PasswordHasher
	tokens  domain.VerificationTokens
	mailer  domain.Mailer
	baseURL string
}

func NewRegisterUser(users domain.UserRepository, hasher domain.PasswordHasher,
	tokens domain.VerificationTokens, mailer domain.Mailer, baseURL string) *RegisterUser {
	return &RegisterUser{users: users, hasher: hasher, tokens: tokens, mailer: mailer, baseURL: baseURL}
}

func (uc *RegisterUser) Execute(ctx context.Context, in RegisterInput) (*domain.User, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, domain.ErrInvalidEmail
	}
	if len(in.Password) < minPasswordLen {
		return nil, domain.ErrWeakPassword
	}

	if _, err := uc.users.FindByEmail(ctx, email); err == nil {
		return nil, domain.ErrUserExists
	} else if err != domain.ErrUserNotFound {
		return nil, err
	}

	hash, err := uc.hasher.Hash(in.Password)
	if err != nil {
		return nil, err
	}

	locale := in.Locale
	if locale == "" {
		locale = "en"
	}
	tz := in.Timezone
	if tz == "" {
		tz = "UTC"
	}

	u := &domain.User{
		ID: uuid.NewString(), Email: email, EmailVerified: false, PasswordHash: hash,
		Name: strings.TrimSpace(in.Name), Locale: locale, Timezone: tz, CreatedAt: time.Now().UTC(),
	}
	if err := uc.users.Create(ctx, u); err != nil {
		return nil, err
	}

	token, err := uc.tokens.Issue(ctx, domain.PurposeVerifyEmail, u.ID, verifyTokenTTL)
	if err != nil {
		return nil, err
	}
	link := uc.baseURL + "/verify-email?token=" + token
	if err := uc.mailer.SendVerification(ctx, u.Email, link); err != nil {
		return nil, err
	}
	return u, nil
}
```
(NOTE: comparison `err != domain.ErrUserNotFound` — use `errors.Is` for wrapped errors. Since fake returns the sentinel directly and the pg repo returns the sentinel unwrapped from `scanOne`, `!=` works, but prefer `!errors.Is(err, domain.ErrUserNotFound)` for safety. Implementer: use `errors.Is`.)

- [ ] **Step 5: Запустить тест — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/application/identity/ -v`
Expected: PASS.

- [ ] **Step 6: Commit** (`go mod tidy` if uuid becomes direct)

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go mod tidy
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/application/identity/ platform/go.mod platform/go.sum
git commit -m "feat(platform): RegisterUser use-case"
```

---

## Task 8: Use-case VerifyEmail

**Files:**
- Create: `platform/internal/application/identity/verify_email.go`
- Test: `platform/internal/application/identity/verify_email_test.go`

- [ ] **Step 1: Написать падающий тест** — `platform/internal/application/identity/verify_email_test.go`:
```go
package identity_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
)

func TestVerifyEmailMarksVerified(t *testing.T) {
	users := newFakeUsers()
	tokens := newFakeTokens()
	// seed an unverified user + a token
	u := &domain.User{ID: "u1", Email: "a@x.com", EmailVerified: false}
	_ = users.Create(context.Background(), u)
	tok, _ := tokens.Issue(context.Background(), domain.PurposeVerifyEmail, "u1", 0)

	uc := identity.NewVerifyEmail(users, tokens)
	require.NoError(t, uc.Execute(context.Background(), tok))

	got, _ := users.FindByID(context.Background(), "u1")
	require.True(t, got.EmailVerified)
}

func TestVerifyEmailRejectsBadToken(t *testing.T) {
	uc := identity.NewVerifyEmail(newFakeUsers(), newFakeTokens())
	err := uc.Execute(context.Background(), "nope")
	require.ErrorIs(t, err, domain.ErrTokenInvalid)
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/application/identity/ -run TestVerifyEmail -v`
Expected: FAIL (нет `NewVerifyEmail`).

- [ ] **Step 3: Реализовать** — `platform/internal/application/identity/verify_email.go`:
```go
package identity

import (
	"context"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

// VerifyEmail consumes a verification token and marks the user's email verified.
type VerifyEmail struct {
	users  domain.UserRepository
	tokens domain.VerificationTokens
}

func NewVerifyEmail(users domain.UserRepository, tokens domain.VerificationTokens) *VerifyEmail {
	return &VerifyEmail{users: users, tokens: tokens}
}

func (uc *VerifyEmail) Execute(ctx context.Context, token string) error {
	userID, err := uc.tokens.Consume(ctx, domain.PurposeVerifyEmail, token)
	if err != nil {
		return err // ErrTokenInvalid
	}
	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	u.EmailVerified = true
	return uc.users.Update(ctx, u)
}
```

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/application/identity/ -run TestVerifyEmail -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/application/identity/
git commit -m "feat(platform): VerifyEmail use-case"
```

---

## Task 9: Use-cases RequestPasswordReset + ResetPassword

**Files:**
- Create: `platform/internal/application/identity/reset_password.go`
- Test: `platform/internal/application/identity/reset_password_test.go`

- [ ] **Step 1: Написать падающий тест** — `platform/internal/application/identity/reset_password_test.go`:
```go
package identity_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
)

func TestRequestPasswordResetSendsMailForExistingUser(t *testing.T) {
	users := newFakeUsers()
	tokens := newFakeTokens()
	mailer := newFakeMailer()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com"})

	uc := identity.NewRequestPasswordReset(users, tokens, mailer, "https://acc.example")
	require.NoError(t, uc.Execute(context.Background(), "A@x.com")) // case-insensitive

	require.Len(t, mailer.resets, 1)
	require.Equal(t, "a@x.com", mailer.resets[0].to)
	require.True(t, strings.Contains(mailer.resets[0].link, tokens.lastToken))
}

func TestRequestPasswordResetSilentForUnknownUser(t *testing.T) {
	mailer := newFakeMailer()
	uc := identity.NewRequestPasswordReset(newFakeUsers(), newFakeTokens(), mailer, "https://acc.example")
	// must NOT error (no account enumeration) and must NOT send mail
	require.NoError(t, uc.Execute(context.Background(), "ghost@x.com"))
	require.Len(t, mailer.resets, 0)
}

func TestResetPasswordSetsNewHash(t *testing.T) {
	users := newFakeUsers()
	tokens := newFakeTokens()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "old"})
	tok, _ := tokens.Issue(context.Background(), domain.PurposePasswordReset, "u1", 0)

	uc := identity.NewResetPassword(users, &fakeHasher{}, tokens)
	require.NoError(t, uc.Execute(context.Background(), tok, "brand-new-pw"))

	got, _ := users.FindByID(context.Background(), "u1")
	require.Equal(t, "hashed:brand-new-pw", got.PasswordHash)
}

func TestResetPasswordRejectsWeak(t *testing.T) {
	uc := identity.NewResetPassword(newFakeUsers(), &fakeHasher{}, newFakeTokens())
	err := uc.Execute(context.Background(), "any", "short")
	require.ErrorIs(t, err, domain.ErrWeakPassword)
}

func TestResetPasswordRejectsBadToken(t *testing.T) {
	uc := identity.NewResetPassword(newFakeUsers(), &fakeHasher{}, newFakeTokens())
	err := uc.Execute(context.Background(), "nope", "brand-new-pw")
	require.ErrorIs(t, err, domain.ErrTokenInvalid)
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/application/identity/ -run "TestRequestPasswordReset|TestResetPassword" -v`
Expected: FAIL (нет конструкторов).

- [ ] **Step 3: Реализовать** — `platform/internal/application/identity/reset_password.go`:
```go
package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

const resetTokenTTL = 1 * time.Hour

// RequestPasswordReset issues a reset token and emails a link. It never reveals
// whether the email exists (no account enumeration).
type RequestPasswordReset struct {
	users   domain.UserRepository
	tokens  domain.VerificationTokens
	mailer  domain.Mailer
	baseURL string
}

func NewRequestPasswordReset(users domain.UserRepository, tokens domain.VerificationTokens,
	mailer domain.Mailer, baseURL string) *RequestPasswordReset {
	return &RequestPasswordReset{users: users, tokens: tokens, mailer: mailer, baseURL: baseURL}
}

func (uc *RequestPasswordReset) Execute(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := uc.users.FindByEmail(ctx, email)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil // silent: do not reveal absence
	}
	if err != nil {
		return err
	}
	token, err := uc.tokens.Issue(ctx, domain.PurposePasswordReset, u.ID, resetTokenTTL)
	if err != nil {
		return err
	}
	link := uc.baseURL + "/reset-password?token=" + token
	return uc.mailer.SendPasswordReset(ctx, u.Email, link)
}

// ResetPassword consumes a reset token and sets a new password hash.
type ResetPassword struct {
	users  domain.UserRepository
	hasher domain.PasswordHasher
	tokens domain.VerificationTokens
}

func NewResetPassword(users domain.UserRepository, hasher domain.PasswordHasher,
	tokens domain.VerificationTokens) *ResetPassword {
	return &ResetPassword{users: users, hasher: hasher, tokens: tokens}
}

func (uc *ResetPassword) Execute(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < minPasswordLen {
		return domain.ErrWeakPassword
	}
	userID, err := uc.tokens.Consume(ctx, domain.PurposePasswordReset, token)
	if err != nil {
		return err // ErrTokenInvalid
	}
	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	hash, err := uc.hasher.Hash(newPassword)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	return uc.users.Update(ctx, u)
}
```
Note: `ResetPassword` checks weak password BEFORE consuming the token, so a weak attempt does not burn the token.

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/application/identity/ -v`
Expected: PASS (все use-case тесты).

- [ ] **Step 5: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/application/identity/
git commit -m "feat(platform): password reset use-cases"
```

---

## Task 10: JSON HTTP-хендлеры identity

**Files:**
- Create: `platform/internal/presentation/http/identity_handlers.go`
- Test: `platform/internal/presentation/http/identity_handlers_test.go`
- Modify: `platform/internal/infrastructure/httpserver/server.go` (NewRouter принимает handlers)

- [ ] **Step 1: Написать падающий тест хендлеров** — `platform/internal/presentation/http/identity_handlers_test.go`:
```go
package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	appidentity "github.com/papyrus/platform/internal/application/identity"
	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

// buildTestServer wires the identity use-cases with in-memory fakes into a chi router.
func buildTestServer(t *testing.T) (*httptest.Server, *fakeMailer) {
	t.Helper()
	users := newFakeUsers()
	tokens := newFakeTokens()
	mailer := newFakeMailer()
	hasher := fakeHasher{}
	base := "https://acc.example"

	h := apphttp.NewIdentityHandlers(
		appidentity.NewRegisterUser(users, hasher, tokens, mailer, base),
		appidentity.NewVerifyEmail(users, tokens),
		appidentity.NewRequestPasswordReset(users, tokens, mailer, base),
		appidentity.NewResetPassword(users, hasher, tokens),
	)
	r := chi.NewRouter()
	h.Register(r)
	return httptest.NewServer(r), mailer
}

func TestRegisterEndpoint(t *testing.T) {
	srv, mailer := buildTestServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"email": "a@x.com", "password": "long-enough-pw", "name": "Al"})
	resp, err := http.Post(srv.URL+"/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.NotEmpty(t, out["id"])
	require.Equal(t, "a@x.com", out["email"])
	require.Len(t, mailer.verifications, 1)
}

func TestRegisterEndpointRejectsWeakPassword(t *testing.T) {
	srv, _ := buildTestServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"email": "a@x.com", "password": "short"})
	resp, err := http.Post(srv.URL+"/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPasswordResetRequestAlwaysAccepted(t *testing.T) {
	srv, _ := buildTestServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"email": "ghost@x.com"})
	resp, err := http.Post(srv.URL+"/password-reset/request", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode) // 202 even for unknown email
}

// fakes reused via a local copy in this package's test files (see fakes_test.go in this package).
var _ = context.Background
```

- [ ] **Step 2: Скопировать фейки в пакет хендлеров** — `platform/internal/presentation/http/fakes_test.go`:

Copy the SAME fake types used in the application tests, but in `package http_test`. Paste this exact content:
```go
package http_test

import (
	"context"
	"time"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

type fakeUsers struct{ byID, byEmail map[string]*domain.User }

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[string]*domain.User{}, byEmail: map[string]*domain.User{}}
}
func (f *fakeUsers) Create(_ context.Context, u *domain.User) error {
	cp := *u
	f.byID[u.ID] = &cp
	f.byEmail[u.Email] = &cp
	return nil
}
func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := f.byEmail[email]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrUserNotFound
}
func (f *fakeUsers) FindByID(_ context.Context, id string) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrUserNotFound
}
func (f *fakeUsers) Update(_ context.Context, u *domain.User) error {
	cp := *u
	f.byID[u.ID] = &cp
	f.byEmail[u.Email] = &cp
	return nil
}

type fakeHasher struct{}

func (fakeHasher) Hash(plain string) (string, error) { return "hashed:" + plain, nil }
func (fakeHasher) Check(hash, plain string) bool      { return hash == "hashed:"+plain }

type fakeTokens struct {
	lastToken string
	store     map[string]string
}

func newFakeTokens() *fakeTokens { return &fakeTokens{store: map[string]string{}} }
func (f *fakeTokens) Issue(_ context.Context, purpose, userID string, _ time.Duration) (string, error) {
	f.lastToken = "tok-" + userID
	f.store[purpose+":"+f.lastToken] = userID
	return f.lastToken, nil
}
func (f *fakeTokens) Consume(_ context.Context, purpose, token string) (string, error) {
	k := purpose + ":" + token
	if uid, ok := f.store[k]; ok {
		delete(f.store, k)
		return uid, nil
	}
	return "", domain.ErrTokenInvalid
}

type sentMail struct{ to, link string }
type fakeMailer struct {
	verifications []sentMail
	resets        []sentMail
}

func newFakeMailer() *fakeMailer { return &fakeMailer{} }
func (f *fakeMailer) SendVerification(_ context.Context, to, link string) error {
	f.verifications = append(f.verifications, sentMail{to, link})
	return nil
}
func (f *fakeMailer) SendPasswordReset(_ context.Context, to, link string) error {
	f.resets = append(f.resets, sentMail{to, link})
	return nil
}
```

- [ ] **Step 3: Запустить — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/presentation/http/ -v`
Expected: FAIL (нет `NewIdentityHandlers`/`Register`).

- [ ] **Step 4: Реализовать хендлеры** — `platform/internal/presentation/http/identity_handlers.go`:
```go
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	appidentity "github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
)

// IdentityHandlers exposes JSON endpoints for registration and password reset.
type IdentityHandlers struct {
	register *appidentity.RegisterUser
	verify   *appidentity.VerifyEmail
	reqReset *appidentity.RequestPasswordReset
	reset    *appidentity.ResetPassword
}

func NewIdentityHandlers(register *appidentity.RegisterUser, verify *appidentity.VerifyEmail,
	reqReset *appidentity.RequestPasswordReset, reset *appidentity.ResetPassword) *IdentityHandlers {
	return &IdentityHandlers{register: register, verify: verify, reqReset: reqReset, reset: reset}
}

// Register mounts the identity routes on r.
func (h *IdentityHandlers) Register(r chi.Router) {
	r.Post("/register", h.handleRegister)
	r.Get("/verify-email", h.handleVerifyEmail)
	r.Post("/password-reset/request", h.handleRequestReset)
	r.Post("/password-reset/confirm", h.handleConfirmReset)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrUserExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "user already exists"})
	case errors.Is(err, domain.ErrWeakPassword):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password too weak"})
	case errors.Is(err, domain.ErrInvalidEmail):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email"})
	case errors.Is(err, domain.ErrTokenInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token invalid or expired"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (h *IdentityHandlers) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email, Password, Name, Locale, Timezone string
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	u, err := h.register.Execute(r.Context(), appidentity.RegisterInput{
		Email: body.Email, Password: body.Password, Name: body.Name,
		Locale: body.Locale, Timezone: body.Timezone,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": u.ID, "email": u.Email})
}

func (h *IdentityHandlers) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if err := h.verify.Execute(r.Context(), token); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

func (h *IdentityHandlers) handleRequestReset(w http.ResponseWriter, r *http.Request) {
	var body struct{ Email string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := h.reqReset.Execute(r.Context(), body.Email); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *IdentityHandlers) handleConfirmReset(w http.ResponseWriter, r *http.Request) {
	var body struct{ Token, Password string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := h.reset.Execute(r.Context(), body.Token, body.Password); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}
```

- [ ] **Step 5: Обновить NewRouter** — `platform/internal/infrastructure/httpserver/server.go`, изменить `NewRouter` чтобы принимал handlers:
```go
package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

// NewRouter wires HTTP routes for the platform.
func NewRouter(identity *apphttp.IdentityHandlers) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", apphttp.Healthz())
	identity.Register(r)

	return r
}

// NewServer builds an *http.Server listening on the given address.
func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: handler,
	}
}
```
Also update `platform/internal/infrastructure/httpserver/server_test.go`: `NewRouter(nil)` would panic when `identity.Register` is called, so pass a real (empty-capable) handler. Change the router test to build a minimal handler set OR just test `/healthz` by mounting only that. Simplest: update the existing test to construct handlers with nil use-cases is unsafe; instead register healthz-only path. Replace the test body with:
```go
func TestRouterServesHealthz(t *testing.T) {
	h := apphttp.NewIdentityHandlers(nil, nil, nil, nil)
	srv := httptest.NewServer(NewRouter(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}
```
(Registering routes whose handlers hold nil use-cases is fine as long as the test only calls `/healthz`, which doesn't touch them. Add the import `apphttp "github.com/papyrus/platform/internal/presentation/http"` to the test file.)

- [ ] **Step 6: Запустить — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/presentation/http/ ./internal/infrastructure/httpserver/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/presentation/http/ platform/internal/infrastructure/httpserver/
git commit -m "feat(platform): identity JSON handlers + router wiring"
```

---

## Task 11: DI-проводка + миграции в main + конфиг

**Files:**
- Modify: `platform/internal/config/config.go` (+ test) — добавить `BaseURL` и mail-настройки
- Modify: `platform/internal/infrastructure/di/wire.go` (+ regen `wire_gen.go`)
- Modify: `platform/cmd/server/main.go` — прогон миграций перед стартом

- [ ] **Step 1: Расширить конфиг тестом** — добавить в `platform/internal/config/config_test.go` новый тест:
```go
func TestLoadReadsBaseURLAndMail(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_NAME", "n")
	t.Setenv("REDIS_HOST", "r")
	t.Setenv("REDIS_PORT", "6379")
	t.Setenv("BASE_URL", "https://acc.example")
	t.Setenv("MAIL_MODE", "log")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "https://acc.example", cfg.BaseURL)
	require.Equal(t, "log", cfg.Mail.Mode)
}
```

- [ ] **Step 2: Запустить — убедиться, что падает**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/config/ -run TestLoadReadsBaseURL -v`
Expected: FAIL (нет `cfg.BaseURL`/`cfg.Mail`).

- [ ] **Step 3: Расширить конфиг** — в `platform/internal/config/config.go` добавить структуру Mail и поля, НЕ ломая существующие required-проверки (BASE_URL/MAIL_* необязательны, с дефолтами):
```go
type MailConfig struct {
	Mode     string // "log" (default) or "smtp"
	Host     string
	Port     string
	User     string
	Password string
	From     string
}

// ... в Config добавить:
//   BaseURL string
//   Mail    MailConfig
```
И в `Load()` после чтения обязательных полей добавить:
```go
	cfg.BaseURL = os.Getenv("BASE_URL")
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:" + cfg.Port
	}
	cfg.Mail = MailConfig{
		Mode:     os.Getenv("MAIL_MODE"),
		Host:     os.Getenv("SMTP_HOST"),
		Port:     os.Getenv("SMTP_PORT"),
		User:     os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     os.Getenv("SMTP_FROM"),
	}
	if cfg.Mail.Mode == "" {
		cfg.Mail.Mode = "log"
	}
```
(place these lines before the final `return cfg, nil`, and after the required-fields check.)

- [ ] **Step 4: Запустить — убедиться, что проходит**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Обновить wire-провайдеры** — `platform/internal/infrastructure/di/wire.go`. Заменить содержимое на (добавлены провайдеры репозитория, токенов, хешера, mailer, use-cases, handlers; `provideServer` теперь зависит от handlers):
```go
//go:build wireinject
// +build wireinject

package di

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/google/wire"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	appidentity "github.com/papyrus/platform/internal/application/identity"
	"github.com/papyrus/platform/internal/config"
	domainidentity "github.com/papyrus/platform/internal/domain/identity"
	"github.com/papyrus/platform/internal/infrastructure/httpserver"
	"github.com/papyrus/platform/internal/infrastructure/mail"
	pgc "github.com/papyrus/platform/internal/infrastructure/postgres"
	rdc "github.com/papyrus/platform/internal/infrastructure/redis"
	"github.com/papyrus/platform/internal/infrastructure/security"
	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

// App holds the wired application dependencies.
type App struct {
	Config config.Config
	DB     *pgxpool.Pool
	Redis  *goredis.Client
	Server *http.Server
}

func provideDB(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	return pgc.Connect(ctx, cfg.DB.DSN())
}

func provideRedis(ctx context.Context, cfg config.Config) (*goredis.Client, error) {
	return rdc.Connect(ctx, cfg.Redis.Addr())
}

func provideUserRepo(pool *pgxpool.Pool) domainidentity.UserRepository {
	return pgc.NewUserRepository(pool)
}

func provideTokens(client *goredis.Client) domainidentity.VerificationTokens {
	return rdc.NewTokenStore(client)
}

func provideHasher() domainidentity.PasswordHasher {
	return security.NewBcryptHasher(0)
}

func provideMailer(cfg config.Config) domainidentity.Mailer {
	if cfg.Mail.Mode == "smtp" {
		return mail.NewSMTPMailer(cfg.Mail.Host, cfg.Mail.Port, cfg.Mail.User, cfg.Mail.Password, cfg.Mail.From)
	}
	return mail.NewLogMailer(log.New(os.Stdout, "", log.LstdFlags))
}

func provideIdentityHandlers(cfg config.Config, users domainidentity.UserRepository,
	hasher domainidentity.PasswordHasher, tokens domainidentity.VerificationTokens,
	mailer domainidentity.Mailer) *apphttp.IdentityHandlers {
	return apphttp.NewIdentityHandlers(
		appidentity.NewRegisterUser(users, hasher, tokens, mailer, cfg.BaseURL),
		appidentity.NewVerifyEmail(users, tokens),
		appidentity.NewRequestPasswordReset(users, tokens, mailer, cfg.BaseURL),
		appidentity.NewResetPassword(users, hasher, tokens),
	)
}

func provideServer(cfg config.Config, identity *apphttp.IdentityHandlers) *http.Server {
	return httpserver.NewServer(":"+cfg.Port, httpserver.NewRouter(identity))
}

// InitializeApp builds the full application graph.
func InitializeApp(ctx context.Context, cfg config.Config) (*App, error) {
	wire.Build(
		provideDB,
		provideRedis,
		provideUserRepo,
		provideTokens,
		provideHasher,
		provideMailer,
		provideIdentityHandlers,
		provideServer,
		wire.Struct(new(App), "*"),
	)
	return nil, nil
}
```

- [ ] **Step 6: Регенерировать wire и собрать**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && make wire && go build ./...`
Expected: `wire_gen.go` перегенерирован, сборка чистая.

- [ ] **Step 7: Прогон миграций в main** — `platform/cmd/server/main.go`, добавить вызов миграций после загрузки конфига и до InitializeApp:
```go
package main

import (
	"context"
	"log"

	"github.com/papyrus/platform/internal/config"
	"github.com/papyrus/platform/internal/infrastructure/di"
	"github.com/papyrus/platform/internal/infrastructure/postgres"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := postgres.RunMigrations(cfg.DB.DSN()); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	app, err := di.InitializeApp(ctx, cfg)
	if err != nil {
		log.Fatalf("init app: %v", err)
	}
	defer app.DB.Close()
	defer func() { _ = app.Redis.Close() }()

	log.Printf("platform-core listening on :%s", cfg.Port)
	if err := app.Server.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
```

- [ ] **Step 8: Собрать и обновить .env.example / compose env**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go build ./...`
Expected: чисто.
Добавить в `platform/.env.example` строки:
```dotenv

# Base URL for links in emails
BASE_URL=http://localhost:8090

# Mail: "log" (dev, prints link) or "smtp"
MAIL_MODE=log
SMTP_HOST=
SMTP_PORT=
SMTP_USER=
SMTP_PASSWORD=
SMTP_FROM=no-reply@papyrus.local
```
Добавить в `platform/docker-compose.yml` в `environment` сервиса `platform-core`:
```yaml
      - BASE_URL=http://localhost:8090
      - MAIL_MODE=log
```

- [ ] **Step 9: Commit**

```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP
git add platform/internal/config/ platform/internal/infrastructure/di/ platform/cmd/server/ platform/.env.example platform/docker-compose.yml
git commit -m "feat(platform): wire identity, run migrations on boot, mail config"
```

---

## Task 12: Финальная проверка фазы

- [ ] **Step 1: Юнит-тесты (без Docker)**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test -short ./...`
Expected: PASS (domain, security, application/identity, presentation/http, config).

- [ ] **Step 2: Полные тесты (с testcontainers)**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go test ./...`
Expected: PASS (включая postgres migrate/repo, redis token store).

- [ ] **Step 3: vet + build**

Run: `cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && go vet ./... && go build ./...`
Expected: чисто.

- [ ] **Step 4: E2E через Docker (smoke)**

Run:
```bash
cd /Users/denisurevic/Downloads/ББД/OUR_APP/platform && docker compose up -d --build
```
После готовности:
```bash
curl -sf -X POST http://localhost:8090/register -H 'Content-Type: application/json' \
  -d '{"email":"smoke@example.com","password":"long-enough-pw","name":"Smoke"}'
```
Expected: `201` и JSON с `id`+`email`. В логах platform-core (`docker compose logs platform-core`) — строка `[mail] verification to=smoke@example.com link=...` (LogMailer). Затем `docker compose down`.

---

## Definition of Done (Фаза 2a)
- Таблица `users` создаётся миграцией при старте (идемпотентно).
- Регистрация создаёт неверифицированного пользователя (bcrypt-хеш), выдаёт токен, «отправляет» письмо (LogMailer).
- Верификация email по токену помечает `email_verified=true`; токен одноразовый.
- Сброс пароля: запрос не раскрывает наличие аккаунта; подтверждение по токену ставит новый хеш; слабый пароль отклоняется до сжигания токена.
- Все флоу доступны через JSON API и покрыты тестами (use-cases на фейках, репозитории/токены на реальных контейнерах, хендлеры на httptest).
- E2E-smoke через Docker: `POST /register` → 201 + ссылка в логах.
- Все тесты зелёные; `go vet`/`go build` чистые.

## Следующая фаза
Фаза 2b: server-rendered HTML (html/template) для login/register/consent/reset + интеграция с Ory Hydra (login/consent-хендлеры, trusted-клиенты) + сессии с богатой инфой и завершением. Планируется отдельным документом.
