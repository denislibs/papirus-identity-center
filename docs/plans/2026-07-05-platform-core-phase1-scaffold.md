# Platform Core — Фаза 1 (Каркас) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Поднять каркас сервиса Platform Core на Go (clean architecture + DI + Postgres + Redis + Hydra), с работающим `/healthz` и всё в docker-compose.

**Architecture:** Отдельный Go-сервис в монорепо (`platform/`). Чистая архитектура (domain/application/infrastructure/presentation), DI через `google/wire` (compile-time). Postgres (pgx) — наша БД, Redis — кэш/токены, Ory Hydra — OAuth2/OIDC-движок. TDD: юнит-тесты через порты, интеграционные через `testcontainers`.

**Tech Stack:** Go 1.22+, chi (HTTP), pgx/v5 (Postgres), go-redis/v9 (Redis), golang-migrate, google/wire (DI), testify + testcontainers-go (тесты), Ory Hydra, Docker Compose.

**Модуль Go:** `github.com/papyrus/platform`

---

## File Structure (создаётся в этой фазе)

```
platform/
  go.mod
  go.sum
  Makefile
  docker-compose.yml
  .env.example
  cmd/server/main.go                         точка входа
  internal/
    config/config.go                          загрузка конфигурации из env
    config/config_test.go
    infrastructure/
      postgres/postgres.go                    pgxpool-подключение
      postgres/postgres_test.go               интеграционный (testcontainers)
      redis/redis.go                          go-redis-подключение
      redis/redis_test.go                     интеграционный (testcontainers)
      httpserver/server.go                    chi-роутер + сервер
      di/wire.go                              wire-провайдеры
      di/wire_gen.go                          сгенерированный wire (не править руками)
    presentation/http/health.go               хендлер /healthz
    presentation/http/health_test.go
  migrations/                                 golang-migrate (пусто на старте)
    .gitkeep
```

---

## Task 1: Инициализация Go-модуля и скелета

**Files:**
- Create: `platform/go.mod`
- Create: `platform/Makefile`
- Create: `platform/.env.example`
- Create: `platform/migrations/.gitkeep`

- [ ] **Step 1: Создать директорию и инициализировать модуль**

Run:
```bash
mkdir -p platform && cd platform
go mod init github.com/papyrus/platform
```
Expected: создан `platform/go.mod` с `module github.com/papyrus/platform` и строкой `go 1.22` (или выше).

- [ ] **Step 2: Добавить базовые зависимости**

Run (из `platform/`):
```bash
go get github.com/go-chi/chi/v5@latest
go get github.com/jackc/pgx/v5@latest
go get github.com/redis/go-redis/v9@latest
go get github.com/google/wire/cmd/wire@latest
go get github.com/stretchr/testify@latest
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/postgres@latest
go get github.com/testcontainers/testcontainers-go/modules/redis@latest
```
Expected: зависимости записаны в `go.mod`/`go.sum`.

- [ ] **Step 3: Создать `platform/.env.example`**

```dotenv
# Platform Core
PORT=8090

# Postgres (наша БД)
DB_HOST=localhost
DB_PORT=5440
DB_USER=platform
DB_PASSWORD=platform
DB_NAME=platform

# Redis
REDIS_HOST=localhost
REDIS_PORT=6390

# Ory Hydra
HYDRA_ADMIN_URL=http://localhost:4445
HYDRA_PUBLIC_URL=http://localhost:4444
```

- [ ] **Step 4: Создать `platform/Makefile`**

```makefile
.PHONY: test test-unit run wire build

test:
	go test ./...

test-unit:
	go test -short ./...

run:
	go run ./cmd/server

wire:
	go run github.com/google/wire/cmd/wire ./internal/infrastructure/di

build:
	go build -o bin/server ./cmd/server
```

- [ ] **Step 5: Создать `platform/migrations/.gitkeep`**

```
```
(пустой файл — чтобы директория попала в git)

- [ ] **Step 6: Commit**

```bash
cd .. && git add platform/go.mod platform/go.sum platform/Makefile platform/.env.example platform/migrations/.gitkeep
git commit -m "chore(platform): init Go module and skeleton"
```

---

## Task 2: Загрузка конфигурации из env

**Files:**
- Create: `platform/internal/config/config.go`
- Test: `platform/internal/config/config_test.go`

- [ ] **Step 1: Написать падающий тест**

`platform/internal/config/config_test.go`:
```go
package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadReadsEnv(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("DB_HOST", "db.example")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_NAME", "n")
	t.Setenv("REDIS_HOST", "redis.example")
	t.Setenv("REDIS_PORT", "6379")

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, "9999", cfg.Port)
	require.Equal(t, "db.example", cfg.DB.Host)
	require.Equal(t, "redis.example", cfg.Redis.Host)
	require.Equal(t, "postgres://u:p@db.example:5432/n?sslmode=disable", cfg.DB.DSN())
}

func TestLoadFailsOnMissingRequired(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DB_HOST", "")
	_, err := Load()
	require.Error(t, err)
}
```

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `cd platform && go test ./internal/config/ -run TestLoad -v`
Expected: FAIL (пакет не компилируется — нет `Load`/`Config`).

- [ ] **Step 3: Реализовать конфиг**

`platform/internal/config/config.go`:
```go
package config

import (
	"fmt"
	"os"
)

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

func (c DBConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.User, c.Password, c.Host, c.Port, c.Name)
}

type RedisConfig struct {
	Host string
	Port string
}

func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

type HydraConfig struct {
	AdminURL  string
	PublicURL string
}

type Config struct {
	Port  string
	DB    DBConfig
	Redis RedisConfig
	Hydra HydraConfig
}

func Load() (Config, error) {
	cfg := Config{
		Port: os.Getenv("PORT"),
		DB: DBConfig{
			Host:     os.Getenv("DB_HOST"),
			Port:     os.Getenv("DB_PORT"),
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASSWORD"),
			Name:     os.Getenv("DB_NAME"),
		},
		Redis: RedisConfig{
			Host: os.Getenv("REDIS_HOST"),
			Port: os.Getenv("REDIS_PORT"),
		},
		Hydra: HydraConfig{
			AdminURL:  os.Getenv("HYDRA_ADMIN_URL"),
			PublicURL: os.Getenv("HYDRA_PUBLIC_URL"),
		},
	}

	if cfg.Port == "" || cfg.DB.Host == "" || cfg.DB.Port == "" ||
		cfg.DB.User == "" || cfg.DB.Name == "" || cfg.Redis.Host == "" ||
		cfg.Redis.Port == "" {
		return Config{}, fmt.Errorf("config: missing required environment variables")
	}
	return cfg, nil
}
```

- [ ] **Step 4: Запустить тест — убедиться, что проходит**

Run: `cd platform && go test ./internal/config/ -v`
Expected: PASS (оба теста).

- [ ] **Step 5: Commit**

```bash
cd .. && git add platform/internal/config/
git commit -m "feat(platform): config loader from env"
```

---

## Task 3: HTTP-сервер и хендлер /healthz

**Files:**
- Create: `platform/internal/presentation/http/health.go`
- Test: `platform/internal/presentation/http/health_test.go`
- Create: `platform/internal/infrastructure/httpserver/server.go`
- Test: `platform/internal/infrastructure/httpserver/server_test.go`

- [ ] **Step 1: Написать падающий тест хендлера**

`platform/internal/presentation/http/health_test.go`:
```go
package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthzReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	Healthz().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, "ok", body["status"])
}
```

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `cd platform && go test ./internal/presentation/http/ -v`
Expected: FAIL (нет `Healthz`).

- [ ] **Step 3: Реализовать хендлер**

`platform/internal/presentation/http/health.go`:
```go
package http

import (
	"encoding/json"
	"net/http"
)

// Healthz returns a liveness handler that reports the service is up.
func Healthz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}
```

- [ ] **Step 4: Запустить тест — убедиться, что проходит**

Run: `cd platform && go test ./internal/presentation/http/ -v`
Expected: PASS.

- [ ] **Step 5: Реализовать сборку роутера/сервера**

`platform/internal/infrastructure/httpserver/server.go`:
```go
package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

// NewRouter wires HTTP routes for the platform.
func NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", apphttp.Healthz())

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

- [ ] **Step 6: Написать тест роутера**

Append to `platform/internal/infrastructure/httpserver/server_test.go`:
```go
package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRouterServesHealthz(t *testing.T) {
	srv := httptest.NewServer(NewRouter())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}
```

- [ ] **Step 7: Запустить тесты — убедиться, что проходят**

Run: `cd platform && go test ./internal/... -v`
Expected: PASS (config, health, httpserver).

- [ ] **Step 8: Commit**

```bash
cd .. && git add platform/internal/presentation/http/ platform/internal/infrastructure/httpserver/
git commit -m "feat(platform): chi router with /healthz"
```

---

## Task 4: Подключение к Postgres (pgxpool)

**Files:**
- Create: `platform/internal/infrastructure/postgres/postgres.go`
- Test: `platform/internal/infrastructure/postgres/postgres_test.go`

- [ ] **Step 1: Написать падающий интеграционный тест (testcontainers)**

`platform/internal/infrastructure/postgres/postgres_test.go`:
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

func TestConnectPings(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()

	container, err := tcpostgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16-alpine"),
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

	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	require.NoError(t, pool.Ping(ctx))
}
```

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `cd platform && go test ./internal/infrastructure/postgres/ -v`
Expected: FAIL (нет `Connect`).

- [ ] **Step 3: Реализовать подключение**

`platform/internal/infrastructure/postgres/postgres.go`:
```go
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgx connection pool and verifies it with a ping.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return pool, nil
}
```

- [ ] **Step 4: Запустить тест — убедиться, что проходит**

Run: `cd platform && go test ./internal/infrastructure/postgres/ -v`
Expected: PASS (требует запущенный Docker для testcontainers).

- [ ] **Step 5: Commit**

```bash
cd .. && git add platform/internal/infrastructure/postgres/
git commit -m "feat(platform): postgres pgxpool connection"
```

---

## Task 5: Подключение к Redis (go-redis)

**Files:**
- Create: `platform/internal/infrastructure/redis/redis.go`
- Test: `platform/internal/infrastructure/redis/redis_test.go`

- [ ] **Step 1: Написать падающий интеграционный тест**

`platform/internal/infrastructure/redis/redis_test.go`:
```go
package redis

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func TestConnectPings(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()

	container, err := tcredis.RunContainer(ctx,
		testcontainers.WithImage("redis:7-alpine"))
	require.NoError(t, err)
	defer func() { _ = container.Terminate(ctx) }()

	endpoint, err := container.Endpoint(ctx, "")
	require.NoError(t, err)

	client, err := Connect(ctx, endpoint)
	require.NoError(t, err)
	defer client.Close()

	require.NoError(t, client.Ping(ctx).Err())
}
```

- [ ] **Step 2: Запустить тест — убедиться, что падает**

Run: `cd platform && go test ./internal/infrastructure/redis/ -v`
Expected: FAIL (нет `Connect`).

- [ ] **Step 3: Реализовать подключение**

`platform/internal/infrastructure/redis/redis.go`:
```go
package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Connect creates a Redis client and verifies it with a ping.
func Connect(ctx context.Context, addr string) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: ping: %w", err)
	}
	return client, nil
}
```

- [ ] **Step 4: Запустить тест — убедиться, что проходит**

Run: `cd platform && go test ./internal/infrastructure/redis/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd .. && git add platform/internal/infrastructure/redis/
git commit -m "feat(platform): redis connection"
```

---

## Task 6: DI-сборка через google/wire

**Files:**
- Create: `platform/internal/infrastructure/di/wire.go`
- Create (generated): `platform/internal/infrastructure/di/wire_gen.go`

- [ ] **Step 1: Написать провайдеры wire**

`platform/internal/infrastructure/di/wire.go`:
```go
//go:build wireinject
// +build wireinject

package di

import (
	"context"
	"net/http"

	"github.com/google/wire"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/papyrus/platform/internal/config"
	"github.com/papyrus/platform/internal/infrastructure/httpserver"
	pgc "github.com/papyrus/platform/internal/infrastructure/postgres"
	rdc "github.com/papyrus/platform/internal/infrastructure/redis"
)

// App holds the wired application dependencies.
type App struct {
	Config config.Config
	DB     *pgxpool.Pool
	Redis  *redis.Client
	Server *http.Server
}

func provideDB(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	return pgc.Connect(ctx, cfg.DB.DSN())
}

func provideRedis(ctx context.Context, cfg config.Config) (*redis.Client, error) {
	return rdc.Connect(ctx, cfg.Redis.Addr())
}

func provideServer(cfg config.Config) *http.Server {
	return httpserver.NewServer(":"+cfg.Port, httpserver.NewRouter())
}

// InitializeApp builds the full application graph.
func InitializeApp(ctx context.Context, cfg config.Config) (*App, error) {
	wire.Build(
		provideDB,
		provideRedis,
		provideServer,
		wire.Struct(new(App), "*"),
	)
	return nil, nil
}
```

- [ ] **Step 2: Сгенерировать wire_gen.go**

Run: `cd platform && make wire`
Expected: создан `platform/internal/infrastructure/di/wire_gen.go` без ошибок.

- [ ] **Step 3: Проверить компиляцию**

Run: `cd platform && go build ./...`
Expected: сборка без ошибок.

- [ ] **Step 4: Commit**

```bash
cd .. && git add platform/internal/infrastructure/di/
git commit -m "feat(platform): DI wiring with google/wire"
```

---

## Task 7: Точка входа main

**Files:**
- Create: `platform/cmd/server/main.go`

- [ ] **Step 1: Реализовать main**

`platform/cmd/server/main.go`:
```go
package main

import (
	"context"
	"log"

	"github.com/papyrus/platform/internal/config"
	"github.com/papyrus/platform/internal/infrastructure/di"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
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

- [ ] **Step 2: Проверить компиляцию**

Run: `cd platform && go build ./cmd/server`
Expected: сборка без ошибок, создан бинарь.

- [ ] **Step 3: Commit**

```bash
cd .. && git add platform/cmd/server/
git commit -m "feat(platform): server entrypoint"
```

---

## Task 8: Docker-обёртка (compose со всем стеком)

**Files:**
- Create: `platform/Dockerfile`
- Create: `platform/docker-compose.yml`

- [ ] **Step 1: Создать Dockerfile**

`platform/Dockerfile`:
```dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /bin/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl
COPY --from=build /bin/server /bin/server
EXPOSE 8090
ENTRYPOINT ["/bin/server"]
```

- [ ] **Step 2: Создать docker-compose.yml**

`platform/docker-compose.yml`:
```yaml
services:
  platform-core:
    build: .
    container_name: platform-core
    ports:
      - "8090:8090"
    environment:
      - PORT=8090
      - DB_HOST=platform-postgres
      - DB_PORT=5432
      - DB_USER=platform
      - DB_PASSWORD=platform
      - DB_NAME=platform
      - REDIS_HOST=platform-redis
      - REDIS_PORT=6379
      - HYDRA_ADMIN_URL=http://hydra:4445
      - HYDRA_PUBLIC_URL=http://hydra:4444
    depends_on:
      platform-postgres:
        condition: service_healthy
      platform-redis:
        condition: service_healthy
      hydra:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:8090/healthz"]
      interval: 15s
      timeout: 5s
      retries: 3
      start_period: 20s
    networks:
      - platform-net

  platform-postgres:
    image: postgres:16-alpine
    container_name: platform-postgres
    environment:
      - POSTGRES_USER=platform
      - POSTGRES_PASSWORD=platform
      - POSTGRES_DB=platform
    ports:
      - "5440:5432"
    volumes:
      - platform_pg:/var/lib/postgresql/data
      - ./scripts/init-hydra-db.sh:/docker-entrypoint-initdb.d/init-hydra-db.sh
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U platform"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - platform-net

  platform-redis:
    image: redis:7-alpine
    container_name: platform-redis
    ports:
      - "6390:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - platform-net

  hydra-migrate:
    image: oryd/hydra:v2.2.0
    container_name: platform-hydra-migrate
    command: migrate sql -e --yes
    environment:
      - DSN=postgres://platform:platform@platform-postgres:5432/hydra?sslmode=disable
    depends_on:
      platform-postgres:
        condition: service_healthy
    restart: on-failure
    networks:
      - platform-net

  hydra:
    image: oryd/hydra:v2.2.0
    container_name: platform-hydra
    command: serve all --dev
    ports:
      - "4444:4444"
      - "4445:4445"
    environment:
      - DSN=postgres://platform:platform@platform-postgres:5432/hydra?sslmode=disable
      - URLS_SELF_ISSUER=http://localhost:4444
      - URLS_LOGIN=http://localhost:8090/login
      - URLS_CONSENT=http://localhost:8090/consent
      - SECRETS_SYSTEM=youReallyNeedToChangeThis
    depends_on:
      hydra-migrate:
        condition: service_completed_successfully
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:4444/health/ready"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 20s
    networks:
      - platform-net

volumes:
  platform_pg:

networks:
  platform-net:
```

- [ ] **Step 3: Создать скрипт инициализации БД Hydra**

`platform/scripts/init-hydra-db.sh`:
```bash
#!/bin/bash
set -e
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<-EOSQL
	CREATE DATABASE hydra;
EOSQL
```
(создаёт вторую логическую БД `hydra` в том же инстансе Postgres — как решено в спеке §6)

- [ ] **Step 4: Поднять стек и проверить healthz**

Run:
```bash
cd platform && docker compose up -d --build
```
Затем дождаться готовности и проверить:
```bash
curl -sf http://localhost:8090/healthz
```
Expected: `{"status":"ok"}`. И `docker compose ps` показывает `platform-core`, `platform-postgres`, `platform-redis`, `hydra` как healthy, `hydra-migrate` — completed.

- [ ] **Step 5: Остановить стек**

Run: `cd platform && docker compose down`
Expected: контейнеры остановлены.

- [ ] **Step 6: Commit**

```bash
cd .. && git add platform/Dockerfile platform/docker-compose.yml platform/scripts/init-hydra-db.sh
git commit -m "feat(platform): dockerize stack (platform-core + postgres + redis + hydra)"
```

---

## Task 9: Финальная проверка фазы

- [ ] **Step 1: Прогнать все юнит-тесты**

Run: `cd platform && make test-unit`
Expected: PASS (config, health, httpserver — без Docker).

- [ ] **Step 2: Прогнать все тесты (с интеграционными)**

Run: `cd platform && make test`
Expected: PASS (включая postgres/redis через testcontainers; требует запущенный Docker).

- [ ] **Step 3: Убедиться, что сборка чистая**

Run: `cd platform && go vet ./... && go build ./...`
Expected: без ошибок и предупреждений.

---

## Definition of Done (Фаза 1)
- `platform/` — Go-модуль с чистой архитектурой, DI через wire.
- `/healthz` отвечает `{"status":"ok"}`.
- Подключения к Postgres и Redis реализованы и покрыты интеграционными тестами.
- Весь стек (platform-core + postgres + redis + hydra + hydra-migrate) поднимается
  через `docker compose up` и проходит healthcheck.
- Все тесты зелёные; `go vet`/`go build` чистые.

## Следующая фаза
Фаза 2 (Identity: users, регистрация, верификация email, логин, сессии + Hydra
login/consent) — планируется отдельным документом перед стартом, опираясь на этот
каркас.
