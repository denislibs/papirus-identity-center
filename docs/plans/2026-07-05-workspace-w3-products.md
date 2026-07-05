# Workspace модуль — W3 (продукты) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Реестр продуктов платформы и включение/выключение продуктов в воркспейсе — через Bearer API.

**Architecture:** Расширяем `workspace` контекст: `Product` (глобальный реестр) + `WorkspaceProduct` (включение). Use-cases: список реестра (любой аутентифицированный), включённые в воркспейсе (участник), включить/выключить (owner/admin). REST под `RequireAuth`. Реестр сидируется миграцией.

**Scope note (W3):** реестр + включение. Продукто-фейсинг read API (members/org-units/positions) уже есть из W1/W2. UI — W4.

**Tech Stack:** Go 1.26, chi, pgx, golang-migrate, testify, testcontainers. Module `github.com/denislibs/papirus-identity-center`.

---

## File Structure
```
internal/
  domain/workspace/
    workspace.go   (+ Product; + ErrProductNotFound)
    ports.go       (+ ProductRepository, WorkspaceProductRepository)
  application/workspace/
    products.go    ListProducts/EnableProduct/DisableProduct/ListEnabledProducts + products_test.go
  infrastructure/postgres/
    migrations/0005_products.up.sql / .down.sql
    workspace_repository.go   (+ ProductRepository, WorkspaceProductRepository)
  presentation/http/
    workspace_handlers.go     (+ product routes; + fakes)
  infrastructure/di/wire.go   (+ провайдеры)
```

---

## Task 1: Миграция products/workspace_products + сид реестра

**Files:** `internal/infrastructure/postgres/migrations/0005_products.up.sql` / `.down.sql`

- [ ] **Step 1: SQL** — `0005_products.up.sql`:
```sql
CREATE TABLE products (
    key  TEXT PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE workspace_products (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    product_key  TEXT NOT NULL REFERENCES products(key) ON DELETE CASCADE,
    enabled_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, product_key)
);

INSERT INTO products (key, name) VALUES
    ('papyrus', 'Papyrus (СЭД)'),
    ('lite', 'Papyrus Lite')
ON CONFLICT (key) DO NOTHING;
```
`0005_products.down.sql`:
```sql
DROP TABLE workspace_products;
DROP TABLE products;
```

- [ ] **Step 2: Тест миграции** — добавить `TestRunMigrationsCreatesProducts` в `migrate_test.go` (по образцу): таблицы `products`/`workspace_products` существуют, и в `products` есть seed-строки (`SELECT count(*) FROM products >= 2`):
```go
func TestRunMigrationsCreatesProducts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("platform"), tcpostgres.WithUsername("platform"), tcpostgres.WithPassword("platform"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	require.NoError(t, err)
	defer func() { _ = container.Terminate(ctx) }()
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, RunMigrations(dsn))
	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM products`).Scan(&n))
	require.GreaterOrEqual(t, n, 2)
	var ok bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name='workspace_products')`).Scan(&ok))
	require.True(t, ok)
}
```

- [ ] **Step 3: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/infrastructure/postgres/ -run TestRunMigrationsCreatesProducts -v` → PASS.

- [ ] **Step 4: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/postgres/migrations/ internal/infrastructure/postgres/migrate_test.go
git commit -m "feat(workspace): products migration + seed"
```

---

## Task 2: Домен — Product + порты

**Files:** `internal/domain/workspace/workspace.go`, `ports.go`

- [ ] **Step 1: Сущность + ошибка** — в `workspace.go`:
```go
type Product struct {
	Key  string
	Name string
}
```
в var-блок:
```go
	ErrProductNotFound = errors.New("workspace: product not found")
```

- [ ] **Step 2: Порты** — в `ports.go`:
```go
type ProductRepository interface {
	ListAll(ctx context.Context) ([]*Product, error)
	Exists(ctx context.Context, key string) (bool, error)
}

type WorkspaceProductRepository interface {
	Enable(ctx context.Context, workspaceID, productKey string) error
	Disable(ctx context.Context, workspaceID, productKey string) error
	ListEnabled(ctx context.Context, workspaceID string) ([]*Product, error)
}
```

- [ ] **Step 3: Сборка** — `cd /Users/denisurevic/Downloads/ББД/platform && go build ./internal/domain/...`

- [ ] **Step 4: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/domain/workspace/
git commit -m "feat(workspace): product entity + ports"
```

---

## Task 3: Postgres — ProductRepository + WorkspaceProductRepository

**Files:** `internal/infrastructure/postgres/workspace_repository.go` (+ test)

- [ ] **Step 1: Падающий тест** — добавить в `workspace_repository_test.go`:
```go
func TestProductRepositories(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	userRepo := NewUserRepository(w.pool)
	wsRepo := NewWorkspaceRepository(w.pool)
	prodRepo := NewProductRepository(w.pool)
	wpRepo := NewWorkspaceProductRepository(w.pool)

	// seeded registry present
	all, err := prodRepo.ListAll(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(all), 2)
	ok, err := prodRepo.Exists(ctx, "papyrus")
	require.NoError(t, err); require.True(t, ok)
	ok, err = prodRepo.Exists(ctx, "nope")
	require.NoError(t, err); require.False(t, ok)

	uid := "ffffffff-0000-0000-0000-000000000001"
	require.NoError(t, userRepo.Create(ctx, &identity.User{ID: uid, Email: "p@x.com", PasswordHash: "h", Locale: "en", Timezone: "UTC", CreatedAt: time.Now().UTC()}))
	wsID := "ffffffff-0000-0000-0000-000000000002"
	require.NoError(t, wsRepo.Create(ctx, &workspace.Workspace{ID: wsID, Name: "P", Slug: "p-1", CreatedBy: uid, CreatedAt: time.Now().UTC()}))

	require.NoError(t, wpRepo.Enable(ctx, wsID, "papyrus"))
	require.NoError(t, wpRepo.Enable(ctx, wsID, "papyrus")) // idempotent
	enabled, err := wpRepo.ListEnabled(ctx, wsID)
	require.NoError(t, err)
	require.Len(t, enabled, 1)
	require.Equal(t, "papyrus", enabled[0].Key)

	require.NoError(t, wpRepo.Disable(ctx, wsID, "papyrus"))
	enabled, err = wpRepo.ListEnabled(ctx, wsID)
	require.NoError(t, err)
	require.Len(t, enabled, 0)
}
```

- [ ] **Step 2: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/infrastructure/postgres/ -run TestProductRepositories -v` → FAIL.

- [ ] **Step 3: Реализовать** — добавить в `workspace_repository.go`:
```go
type ProductRepository struct{ pool *pgxpool.Pool }

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository { return &ProductRepository{pool} }

func (r *ProductRepository) ListAll(ctx context.Context) ([]*workspace.Product, error) {
	rows, err := r.pool.Query(ctx, `SELECT key, name FROM products ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list products: %w", err)
	}
	defer rows.Close()
	var out []*workspace.Product
	for rows.Next() {
		var p workspace.Product
		if err := rows.Scan(&p.Key, &p.Name); err != nil {
			return nil, fmt.Errorf("postgres: scan product: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *ProductRepository) Exists(ctx context.Context, key string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM products WHERE key=$1)`, key).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("postgres: product exists: %w", err)
	}
	return ok, nil
}

type WorkspaceProductRepository struct{ pool *pgxpool.Pool }

func NewWorkspaceProductRepository(pool *pgxpool.Pool) *WorkspaceProductRepository { return &WorkspaceProductRepository{pool} }

func (r *WorkspaceProductRepository) Enable(ctx context.Context, workspaceID, productKey string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO workspace_products (workspace_id, product_key) VALUES ($1,$2) ON CONFLICT DO NOTHING`, workspaceID, productKey)
	if err != nil {
		return fmt.Errorf("postgres: enable product: %w", err)
	}
	return nil
}

func (r *WorkspaceProductRepository) Disable(ctx context.Context, workspaceID, productKey string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM workspace_products WHERE workspace_id=$1 AND product_key=$2`, workspaceID, productKey)
	if err != nil {
		return fmt.Errorf("postgres: disable product: %w", err)
	}
	return nil
}

func (r *WorkspaceProductRepository) ListEnabled(ctx context.Context, workspaceID string) ([]*workspace.Product, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT p.key, p.name FROM products p JOIN workspace_products wp ON wp.product_key = p.key
		 WHERE wp.workspace_id=$1 ORDER BY p.name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list enabled products: %w", err)
	}
	defer rows.Close()
	var out []*workspace.Product
	for rows.Next() {
		var p workspace.Product
		if err := rows.Scan(&p.Key, &p.Name); err != nil {
			return nil, fmt.Errorf("postgres: scan enabled product: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/infrastructure/postgres/ -run TestProductRepositories -v` → PASS.

- [ ] **Step 5: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/postgres/workspace_repository.go internal/infrastructure/postgres/workspace_repository_test.go
git commit -m "feat(workspace): product repositories"
```

---

## Task 4: Use-cases (ListProducts/Enable/Disable/ListEnabled)

**Files:** `internal/application/workspace/products.go` (+ test); extend `fakes_test.go`.

**Авторизация:** ListProducts — любой аутентифицированный (без проверки членства; это глобальный реестр). ListEnabledProducts — участник. Enable/Disable — owner/admin. Enable проверяет, что продукт есть в реестре (Exists → ErrProductNotFound). Использовать существующие `requireManager`/`requireMember`.

- [ ] **Step 1: Фейки** — в `internal/application/workspace/fakes_test.go` добавить:
```go
type fakeProducts struct{ list []*domain.Product }

func (f *fakeProducts) ListAll(_ context.Context) ([]*domain.Product, error) { return f.list, nil }
func (f *fakeProducts) Exists(_ context.Context, key string) (bool, error) {
	for _, p := range f.list { if p.Key == key { return true, nil } }
	return false, nil
}

type fakeWorkspaceProducts struct{ enabled map[string]map[string]bool } // wsID -> key -> true

func newFakeWorkspaceProducts() *fakeWorkspaceProducts { return &fakeWorkspaceProducts{enabled: map[string]map[string]bool{}} }
func (f *fakeWorkspaceProducts) Enable(_ context.Context, wsID, key string) error {
	if f.enabled[wsID] == nil { f.enabled[wsID] = map[string]bool{} }
	f.enabled[wsID][key] = true
	return nil
}
func (f *fakeWorkspaceProducts) Disable(_ context.Context, wsID, key string) error {
	if f.enabled[wsID] != nil { delete(f.enabled[wsID], key) }
	return nil
}
func (f *fakeWorkspaceProducts) ListEnabled(_ context.Context, wsID string) ([]*domain.Product, error) {
	var out []*domain.Product
	for key := range f.enabled[wsID] { out = append(out, &domain.Product{Key: key, Name: key}) }
	return out, nil
}
```

- [ ] **Step 2: Падающий тест** — `internal/application/workspace/products_test.go`:
```go
package workspace_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	appws "github.com/denislibs/papirus-identity-center/internal/application/workspace"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

func TestListProducts(t *testing.T) {
	products := &fakeProducts{list: []*domain.Product{{Key: "papyrus", Name: "Papyrus"}, {Key: "lite", Name: "Lite"}}}
	list, err := appws.NewListProducts(products).Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 2)
}

func TestEnableAndListEnabledProducts(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	w, _ := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Acme")
	products := &fakeProducts{list: []*domain.Product{{Key: "papyrus", Name: "Papyrus"}}}
	wp := newFakeWorkspaceProducts()

	err := appws.NewEnableProduct(members, products, wp).Execute(context.Background(), w.ID, "owner-1", "papyrus")
	require.NoError(t, err)

	list, err := appws.NewListEnabledProducts(members, wp).Execute(context.Background(), w.ID, "owner-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestEnableProductRejectsUnknown(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	w, _ := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Acme")
	err := appws.NewEnableProduct(members, &fakeProducts{}, newFakeWorkspaceProducts()).Execute(context.Background(), w.ID, "owner-1", "ghost")
	require.ErrorIs(t, err, domain.ErrProductNotFound)
}

func TestEnableProductRejectsNonManager(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	w, _ := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Acme")
	_ = members.Create(context.Background(), &domain.Member{ID: "m2", WorkspaceID: w.ID, UserID: "member-2", Role: domain.RoleMember, Status: domain.StatusActive})
	products := &fakeProducts{list: []*domain.Product{{Key: "papyrus", Name: "P"}}}
	err := appws.NewEnableProduct(members, products, newFakeWorkspaceProducts()).Execute(context.Background(), w.ID, "member-2", "papyrus")
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestDisableProduct(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	w, _ := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Acme")
	products := &fakeProducts{list: []*domain.Product{{Key: "papyrus", Name: "P"}}}
	wp := newFakeWorkspaceProducts()
	_ = appws.NewEnableProduct(members, products, wp).Execute(context.Background(), w.ID, "owner-1", "papyrus")
	err := appws.NewDisableProduct(members, wp).Execute(context.Background(), w.ID, "owner-1", "papyrus")
	require.NoError(t, err)
	list, _ := appws.NewListEnabledProducts(members, wp).Execute(context.Background(), w.ID, "owner-1")
	require.Len(t, list, 0)
}

func TestListEnabledRequiresMembership(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	w, _ := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Acme")
	_, err := appws.NewListEnabledProducts(members, newFakeWorkspaceProducts()).Execute(context.Background(), w.ID, "stranger")
	require.ErrorIs(t, err, domain.ErrNotMember)
}
```

- [ ] **Step 3: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/application/workspace/ -run "Product" -v` → FAIL.

- [ ] **Step 4: Реализовать** — `internal/application/workspace/products.go`:
```go
package workspace

import (
	"context"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type ListProducts struct{ products domain.ProductRepository }

func NewListProducts(p domain.ProductRepository) *ListProducts { return &ListProducts{p} }

func (uc *ListProducts) Execute(ctx context.Context) ([]*domain.Product, error) {
	return uc.products.ListAll(ctx)
}

type EnableProduct struct {
	members  domain.MemberRepository
	products domain.ProductRepository
	wp       domain.WorkspaceProductRepository
}

func NewEnableProduct(m domain.MemberRepository, p domain.ProductRepository, wp domain.WorkspaceProductRepository) *EnableProduct {
	return &EnableProduct{members: m, products: p, wp: wp}
}

func (uc *EnableProduct) Execute(ctx context.Context, wsID, requesterID, productKey string) error {
	if err := requireManager(ctx, uc.members, wsID, requesterID); err != nil {
		return err
	}
	ok, err := uc.products.Exists(ctx, productKey)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrProductNotFound
	}
	return uc.wp.Enable(ctx, wsID, productKey)
}

type DisableProduct struct {
	members domain.MemberRepository
	wp      domain.WorkspaceProductRepository
}

func NewDisableProduct(m domain.MemberRepository, wp domain.WorkspaceProductRepository) *DisableProduct {
	return &DisableProduct{members: m, wp: wp}
}

func (uc *DisableProduct) Execute(ctx context.Context, wsID, requesterID, productKey string) error {
	if err := requireManager(ctx, uc.members, wsID, requesterID); err != nil {
		return err
	}
	return uc.wp.Disable(ctx, wsID, productKey)
}

type ListEnabledProducts struct {
	members domain.MemberRepository
	wp      domain.WorkspaceProductRepository
}

func NewListEnabledProducts(m domain.MemberRepository, wp domain.WorkspaceProductRepository) *ListEnabledProducts {
	return &ListEnabledProducts{members: m, wp: wp}
}

func (uc *ListEnabledProducts) Execute(ctx context.Context, wsID, requesterID string) ([]*domain.Product, error) {
	if err := requireMember(ctx, uc.members, wsID, requesterID); err != nil {
		return nil, err
	}
	return uc.wp.ListEnabled(ctx, wsID)
}
```

- [ ] **Step 5: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/application/workspace/ -v` → PASS (все).

- [ ] **Step 6: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/application/workspace/
git commit -m "feat(workspace): product enablement use-cases"
```

---

## Task 5: REST API (products) + DI

**Files:** `internal/presentation/http/workspace_handlers.go` (+ test, + fakes), `internal/infrastructure/di/wire.go`.

**Роуты (RequireAuth):**
- `GET /products` (любой аутентифицированный) → реестр
- `GET /workspaces/{id}/products` (member) → включённые
- `POST /workspaces/{id}/products` {product_key} (owner/admin) → 201
- `DELETE /workspaces/{id}/products/{key}` (owner/admin) → 204

- [ ] **Step 1: Расширить WorkspaceHandlers** — добавить 4 use-case-поля (`*ListProducts, *EnableProduct, *DisableProduct, *ListEnabledProducts`) в struct + конструктор `NewWorkspaceHandlers`; добавить маршруты + методы; расширить `wsErr` кейсом `ErrProductNotFound` → 404.
```go
// в Register:
	r.Get("/products", h.listProducts)
	r.Get("/workspaces/{id}/products", h.listEnabledProducts)
	r.Post("/workspaces/{id}/products", h.enableProduct)
	r.Delete("/workspaces/{id}/products/{key}", h.disableProduct)
```
Пример:
```go
func (h *WorkspaceHandlers) enableProduct(w http.ResponseWriter, r *http.Request) {
	var body struct{ ProductKey string `json:"product_key"` }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := h.enableProductUC.Execute(r.Context(), chi.URLParam(r, "id"), UserIDFromContext(r.Context()), body.ProductKey); err != nil {
		wsErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "enabled"})
}

func (h *WorkspaceHandlers) listProducts(w http.ResponseWriter, r *http.Request) {
	list, err := h.listProductsUC.Execute(r.Context())
	if err != nil { wsErr(w, err); return }
	type dto struct{ Key, Name string }
	out := make([]dto, 0, len(list))
	for _, p := range list { out = append(out, dto{p.Key, p.Name}) }
	writeJSON(w, http.StatusOK, out)
}
```
(`disableProduct` использует `chi.URLParam(r,"key")` → 204; `listEnabledProducts` — member, JSON-массив.)
`wsErr` +:
```go
	case errors.Is(err, domainws.ErrProductNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "product not found"})
```

- [ ] **Step 2: Фейки + тесты** — в `workspace_fakes_test.go` добавить `fakeProductsHTTP` + `fakeWorkspaceProductsHTTP` (по образцу app-фейков). В `workspace_handlers_test.go` обновить `buildWSAPI` под новую сигнатуру конструктора; добавить тесты: `TestListProductsEndpoint` (реестр из фейка), `TestEnableProductEndpoint` (owner → 201, затем GET enabled → 1), `TestEnableProductForbiddenForNonManager`.

- [ ] **Step 3: TDD run** — падает → реализовать → проходит. Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -v`.

- [ ] **Step 4: DI** — в `wire.go`: провайдеры `provideProductRepo`/`provideWorkspaceProductRepo` (→ domainws интерфейсы через `pgc.NewProductRepository`/`NewWorkspaceProductRepository`); обновить `provideWorkspaceHandlers`, чтобы строил и передавал 4 новых use-case. `make wire && go build ./... && go vet ./... && go test -short ./...`. Обновить `server_test.go` под новую сигнатуру `NewWorkspaceHandlers` (добавить nil-аргументы).

- [ ] **Step 5: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/presentation/http/ internal/infrastructure/di/
git commit -m "feat(workspace): product enablement REST API + wiring"
```

---

## Task 6: Финальная проверка + push
- [ ] **Step 1:** `cd /Users/denisurevic/Downloads/ББД/platform && go test ./... && go vet ./... && go build ./...` → зелёно/чисто.
- [ ] **Step 2:** `git push origin main`.

---

## Definition of Done (W3)
- Миграция products (+ seed papyrus/lite) + workspace_products.
- Репо + use-cases: список реестра, включённые в воркспейсе (участник), включить/выключить (owner/admin, включение проверяет наличие в реестре, идемпотентно).
- REST под Bearer-auth; авторизация покрыта тестами.
- Все тесты зелёные; vet/build чисто; запушено.

## Следующая фаза
W4: UI в хабе — свитчер воркспейсов, создание воркспейса, страницы участников/структуры/продуктов (server-rendered поверх use-cases in-process по hub-subject).
