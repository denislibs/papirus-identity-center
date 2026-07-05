# Workspace модуль — W2 (оргструктура) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Оргструктура воркспейса: подразделения (дерево) + должности, и назначение участников на подразделение/должность — через Bearer API.

**Architecture:** Расширяем `workspace` контекст: сущности `OrgUnit`/`Position`, поля `OrgUnitID`/`PositionID` у `Member`. Use-cases с авторизацией (чтение — участник; мутации — owner/admin). REST под `RequireAuth`.

**Scope note (W2):** оргструктура + назначения. НЕ входит: продукты (→ W3), UI (→ W4). Общая, без продуктовой специфики (§6).

**Tech Stack:** Go 1.26, chi, pgx, golang-migrate, testify, testcontainers. Module `github.com/denislibs/papirus-identity-center`.

---

## File Structure
```
internal/
  domain/workspace/
    workspace.go        (+ OrgUnit, Position; + Member.OrgUnitID/PositionID; + ErrOrgUnitNotFound/ErrPositionNotFound)
    ports.go            (+ OrgUnitRepository, PositionRepository; + MemberRepository.Assign)
  application/workspace/
    org_structure.go    CreateOrgUnit/ListOrgUnits/CreatePosition/ListPositions/AssignMember + org_structure_test.go
  infrastructure/postgres/
    migrations/0004_org_structure.up.sql / .down.sql
    workspace_repository.go   (+ OrgUnitRepository, PositionRepository; update Member scans + Assign)
  presentation/http/
    workspace_handlers.go     (+ org-units/positions/assignment routes)
  infrastructure/di/wire.go   (+ провайдеры)
```

---

## Task 1: Миграция org_units/positions + колонки у members

**Files:** `internal/infrastructure/postgres/migrations/0004_org_structure.up.sql` / `.down.sql`

- [ ] **Step 1: SQL** — `0004_org_structure.up.sql`:
```sql
CREATE TABLE org_units (
    id           UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    parent_id    UUID REFERENCES org_units(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_org_units_ws ON org_units (workspace_id);

CREATE TABLE positions (
    id           UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    title        TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_positions_ws ON positions (workspace_id);

ALTER TABLE workspace_members
    ADD COLUMN org_unit_id UUID REFERENCES org_units(id) ON DELETE SET NULL,
    ADD COLUMN position_id UUID REFERENCES positions(id) ON DELETE SET NULL;
```
`0004_org_structure.down.sql`:
```sql
ALTER TABLE workspace_members DROP COLUMN position_id;
ALTER TABLE workspace_members DROP COLUMN org_unit_id;
DROP TABLE positions;
DROP TABLE org_units;
```

- [ ] **Step 2: Тест миграции** — добавить в `migrate_test.go` `TestRunMigrationsCreatesOrgStructure` (по образцу `TestRunMigrationsCreatesWorkspaces`): проверить существование таблиц `org_units`, `positions` и наличие колонок `org_unit_id`/`position_id` у `workspace_members`:
```go
func TestRunMigrationsCreatesOrgStructure(t *testing.T) {
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
	for _, tbl := range []string{"org_units", "positions"} {
		var ok bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name=$1)`, tbl).Scan(&ok))
		require.True(t, ok, tbl)
	}
	for _, col := range []string{"org_unit_id", "position_id"} {
		var ok bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.columns WHERE table_name='workspace_members' AND column_name=$1)`, col).Scan(&ok))
		require.True(t, ok, col)
	}
}
```

- [ ] **Step 3: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/infrastructure/postgres/ -run TestRunMigrationsCreatesOrgStructure -v` → PASS.

- [ ] **Step 4: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/postgres/migrations/ internal/infrastructure/postgres/migrate_test.go
git commit -m "feat(workspace): org structure migration"
```

---

## Task 2: Домен — OrgUnit/Position + Member-поля + порты

**Files:** `internal/domain/workspace/workspace.go`, `ports.go`

- [ ] **Step 1: Сущности** — в `workspace.go` добавить:
```go
type OrgUnit struct {
	ID          string
	WorkspaceID string
	ParentID    *string
	Name        string
	SortOrder   int
	CreatedAt   time.Time
}

type Position struct {
	ID          string
	WorkspaceID string
	Title       string
	CreatedAt   time.Time
}
```
В `Member` добавить поля (nullable):
```go
	OrgUnitID  *string
	PositionID *string
```
В var-блок ошибок добавить:
```go
	ErrOrgUnitNotFound  = errors.New("workspace: org unit not found")
	ErrPositionNotFound = errors.New("workspace: position not found")
	ErrInvalidTitle     = errors.New("workspace: invalid title")
```

- [ ] **Step 2: Порты** — в `ports.go` добавить интерфейсы + расширить MemberRepository:
```go
type OrgUnitRepository interface {
	Create(ctx context.Context, u *OrgUnit) error
	ListByWorkspace(ctx context.Context, workspaceID string) ([]*OrgUnit, error)
	Exists(ctx context.Context, workspaceID, id string) (bool, error)
}

type PositionRepository interface {
	Create(ctx context.Context, p *Position) error
	ListByWorkspace(ctx context.Context, workspaceID string) ([]*Position, error)
	Exists(ctx context.Context, workspaceID, id string) (bool, error)
}
```
В `MemberRepository` добавить метод:
```go
	// Assign sets a member's org unit and position (either may be nil to clear).
	Assign(ctx context.Context, workspaceID, userID string, orgUnitID, positionID *string) error
```

- [ ] **Step 3: Сборка** — `cd /Users/denisurevic/Downloads/ББД/platform && go build ./internal/domain/...` (репозитории пока не реализуют новый метод — это ок для домена; реализация в Task 3).

- [ ] **Step 4: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/domain/workspace/
git commit -m "feat(workspace): org unit/position entities + ports"
```

---

## Task 3: Postgres — OrgUnit/Position репо + Member.Assign + обновить Member-сканы

**Files:** `internal/infrastructure/postgres/workspace_repository.go` (+ test)

**ВАЖНО:** `Member` теперь имеет `OrgUnitID`/`PositionID`. Нужно обновить существующие `MemberRepository.Create` (INSERT), `Find`/`ListByWorkspace` (SELECT + Scan) чтобы включить `org_unit_id, position_id`, и добавить `Assign`. Сканировать nullable-колонки в `*string` (pgx поддерживает `*string` для NULL).

- [ ] **Step 1: Падающий тест** — добавить в `workspace_repository_test.go`:
```go
func TestOrgUnitPositionAndAssign(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	userRepo := NewUserRepository(w.pool)
	wsRepo := NewWorkspaceRepository(w.pool)
	memRepo := NewMemberRepository(w.pool)
	unitRepo := NewOrgUnitRepository(w.pool)
	posRepo := NewPositionRepository(w.pool)

	uid := "99999999-9999-9999-9999-999999999999"
	require.NoError(t, userRepo.Create(ctx, &identity.User{ID: uid, Email: "u2@x.com", PasswordHash: "h", Locale: "en", Timezone: "UTC", CreatedAt: time.Now().UTC()}))
	wsID := "aaaaaaaa-0000-0000-0000-000000000001"
	require.NoError(t, wsRepo.Create(ctx, &workspace.Workspace{ID: wsID, Name: "Org", Slug: "org-1", CreatedBy: uid, CreatedAt: time.Now().UTC()}))
	require.NoError(t, memRepo.Create(ctx, &workspace.Member{ID: "bbbbbbbb-0000-0000-0000-000000000001", WorkspaceID: wsID, UserID: uid, Role: workspace.RoleOwner, Status: workspace.StatusActive, CreatedAt: time.Now().UTC()}))

	unit := &workspace.OrgUnit{ID: "cccccccc-0000-0000-0000-000000000001", WorkspaceID: wsID, Name: "Sales", SortOrder: 1, CreatedAt: time.Now().UTC()}
	require.NoError(t, unitRepo.Create(ctx, unit))
	child := &workspace.OrgUnit{ID: "cccccccc-0000-0000-0000-000000000002", WorkspaceID: wsID, ParentID: &unit.ID, Name: "West", CreatedAt: time.Now().UTC()}
	require.NoError(t, unitRepo.Create(ctx, child))
	units, err := unitRepo.ListByWorkspace(ctx, wsID)
	require.NoError(t, err)
	require.Len(t, units, 2)
	ok, err := unitRepo.Exists(ctx, wsID, unit.ID)
	require.NoError(t, err); require.True(t, ok)

	pos := &workspace.Position{ID: "dddddddd-0000-0000-0000-000000000001", WorkspaceID: wsID, Title: "Manager", CreatedAt: time.Now().UTC()}
	require.NoError(t, posRepo.Create(ctx, pos))
	positions, err := posRepo.ListByWorkspace(ctx, wsID)
	require.NoError(t, err)
	require.Len(t, positions, 1)

	require.NoError(t, memRepo.Assign(ctx, wsID, uid, &unit.ID, &pos.ID))
	m, err := memRepo.Find(ctx, wsID, uid)
	require.NoError(t, err)
	require.NotNil(t, m.OrgUnitID); require.Equal(t, unit.ID, *m.OrgUnitID)
	require.NotNil(t, m.PositionID); require.Equal(t, pos.ID, *m.PositionID)
}
```

- [ ] **Step 2: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/infrastructure/postgres/ -run TestOrgUnitPositionAndAssign -v` → FAIL.

- [ ] **Step 3: Реализовать.**
Обновить `MemberRepository` в `workspace_repository.go`:
- `Create` INSERT: добавить колонки `org_unit_id, position_id` и значения `m.OrgUnitID, m.PositionID` (позиции $7,$8):
```go
func (r *MemberRepository) Create(ctx context.Context, m *workspace.Member) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO workspace_members (id, workspace_id, user_id, role, status, created_at, org_unit_id, position_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		m.ID, m.WorkspaceID, m.UserID, m.Role, m.Status, m.CreatedAt, m.OrgUnitID, m.PositionID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return workspace.ErrAlreadyMember
		}
		return fmt.Errorf("postgres: create member: %w", err)
	}
	return nil
}
```
- `Find` и `ListByWorkspace`: добавить `org_unit_id, position_id` в SELECT и в `Scan(..., &m.OrgUnitID, &m.PositionID)`:
```go
func (r *MemberRepository) Find(ctx context.Context, workspaceID, userID string) (*workspace.Member, error) {
	var m workspace.Member
	err := r.pool.QueryRow(ctx, `SELECT id, workspace_id, user_id, role, status, created_at, org_unit_id, position_id FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, userID).
		Scan(&m.ID, &m.WorkspaceID, &m.UserID, &m.Role, &m.Status, &m.CreatedAt, &m.OrgUnitID, &m.PositionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, workspace.ErrNotMember
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find member: %w", err)
	}
	return &m, nil
}
```
(аналогично `ListByWorkspace`: SELECT + Scan с двумя новыми полями.)
- Добавить `Assign`:
```go
func (r *MemberRepository) Assign(ctx context.Context, workspaceID, userID string, orgUnitID, positionID *string) error {
	_, err := r.pool.Exec(ctx, `UPDATE workspace_members SET org_unit_id=$3, position_id=$4 WHERE workspace_id=$1 AND user_id=$2`,
		workspaceID, userID, orgUnitID, positionID)
	if err != nil {
		return fmt.Errorf("postgres: assign member: %w", err)
	}
	return nil
}
```
Добавить новые репозитории:
```go
type OrgUnitRepository struct{ pool *pgxpool.Pool }

func NewOrgUnitRepository(pool *pgxpool.Pool) *OrgUnitRepository { return &OrgUnitRepository{pool} }

func (r *OrgUnitRepository) Create(ctx context.Context, u *workspace.OrgUnit) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO org_units (id, workspace_id, parent_id, name, sort_order, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		u.ID, u.WorkspaceID, u.ParentID, u.Name, u.SortOrder, u.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create org unit: %w", err)
	}
	return nil
}

func (r *OrgUnitRepository) ListByWorkspace(ctx context.Context, workspaceID string) ([]*workspace.OrgUnit, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, workspace_id, parent_id, name, sort_order, created_at FROM org_units WHERE workspace_id=$1 ORDER BY sort_order, name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list org units: %w", err)
	}
	defer rows.Close()
	var out []*workspace.OrgUnit
	for rows.Next() {
		var u workspace.OrgUnit
		if err := rows.Scan(&u.ID, &u.WorkspaceID, &u.ParentID, &u.Name, &u.SortOrder, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan org unit: %w", err)
		}
		out = append(out, &u)
	}
	return out, rows.Err()
}

func (r *OrgUnitRepository) Exists(ctx context.Context, workspaceID, id string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org_units WHERE workspace_id=$1 AND id=$2)`, workspaceID, id).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("postgres: org unit exists: %w", err)
	}
	return ok, nil
}

type PositionRepository struct{ pool *pgxpool.Pool }

func NewPositionRepository(pool *pgxpool.Pool) *PositionRepository { return &PositionRepository{pool} }

func (r *PositionRepository) Create(ctx context.Context, p *workspace.Position) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO positions (id, workspace_id, title, created_at) VALUES ($1,$2,$3,$4)`,
		p.ID, p.WorkspaceID, p.Title, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create position: %w", err)
	}
	return nil
}

func (r *PositionRepository) ListByWorkspace(ctx context.Context, workspaceID string) ([]*workspace.Position, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, workspace_id, title, created_at FROM positions WHERE workspace_id=$1 ORDER BY title`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list positions: %w", err)
	}
	defer rows.Close()
	var out []*workspace.Position
	for rows.Next() {
		var p workspace.Position
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.Title, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan position: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *PositionRepository) Exists(ctx context.Context, workspaceID, id string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM positions WHERE workspace_id=$1 AND id=$2)`, workspaceID, id).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("postgres: position exists: %w", err)
	}
	return ok, nil
}
```
**NB:** the W1 `workspace_repository_test.go` `TestWorkspaceRepositories` created a member and asserted nothing about org/position — it still passes (new columns are nullable, scans return nil). Verify it stays green.

- [ ] **Step 4: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/infrastructure/postgres/ -run "TestOrgUnitPositionAndAssign|TestWorkspaceRepositories" -v` → PASS.

- [ ] **Step 5: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/postgres/workspace_repository.go internal/infrastructure/postgres/workspace_repository_test.go
git commit -m "feat(workspace): org unit/position repos + member assignment"
```

---

## Task 4: Use-cases (оргструктура + назначение)

**Files:** `internal/application/workspace/org_structure.go` (+ test); extend `fakes_test.go`.

**Авторизация:** чтение (ListOrgUnits/ListPositions) — участник (`members.Find` → ErrNotMember). Мутации (Create*, AssignMember) — owner/admin (`CanManageMembers`). AssignMember дополнительно проверяет, что переданные orgUnitID/positionID (если не nil) принадлежат этому воркспейсу (Exists → ErrOrgUnitNotFound/ErrPositionNotFound), и что целевой пользователь — участник.

- [ ] **Step 1: Расширить фейки** — в `internal/application/workspace/fakes_test.go` добавить `fakeOrgUnits`, `fakePositions` (in-memory реализации портов) и метод `Assign` у существующего `fakeMembers`:
```go
type fakeOrgUnits struct{ list []*domain.OrgUnit }

func (f *fakeOrgUnits) Create(_ context.Context, u *domain.OrgUnit) error { cp := *u; f.list = append(f.list, &cp); return nil }
func (f *fakeOrgUnits) ListByWorkspace(_ context.Context, wsID string) ([]*domain.OrgUnit, error) {
	var out []*domain.OrgUnit
	for _, u := range f.list { if u.WorkspaceID == wsID { cp := *u; out = append(out, &cp) } }
	return out, nil
}
func (f *fakeOrgUnits) Exists(_ context.Context, wsID, id string) (bool, error) {
	for _, u := range f.list { if u.WorkspaceID == wsID && u.ID == id { return true, nil } }
	return false, nil
}

type fakePositions struct{ list []*domain.Position }

func (f *fakePositions) Create(_ context.Context, p *domain.Position) error { cp := *p; f.list = append(f.list, &cp); return nil }
func (f *fakePositions) ListByWorkspace(_ context.Context, wsID string) ([]*domain.Position, error) {
	var out []*domain.Position
	for _, p := range f.list { if p.WorkspaceID == wsID { cp := *p; out = append(out, &cp) } }
	return out, nil
}
func (f *fakePositions) Exists(_ context.Context, wsID, id string) (bool, error) {
	for _, p := range f.list { if p.WorkspaceID == wsID && p.ID == id { return true, nil } }
	return false, nil
}
```
И добавить к `fakeMembers` метод:
```go
func (f *fakeMembers) Assign(_ context.Context, wsID, userID string, orgUnitID, positionID *string) error {
	for _, m := range f.list {
		if m.WorkspaceID == wsID && m.UserID == userID {
			m.OrgUnitID = orgUnitID
			m.PositionID = positionID
			return nil
		}
	}
	return domain.ErrNotMember
}
```

- [ ] **Step 2: Падающий тест** — `internal/application/workspace/org_structure_test.go`:
```go
package workspace_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	appws "github.com/denislibs/papirus-identity-center/internal/application/workspace"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

func setupWS(t *testing.T) (*fakeWS, *fakeMembers, *fakeOrgUnits, *fakePositions, string) {
	t.Helper()
	members := newFakeMembers()
	ws := newFakeWS(members)
	w, err := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Acme")
	require.NoError(t, err)
	return ws, members, &fakeOrgUnits{}, &fakePositions{}, w.ID
}

func TestCreateAndListOrgUnits(t *testing.T) {
	_, members, units, _, wsID := setupWS(t)
	create := appws.NewCreateOrgUnit(members, units)
	_, err := create.Execute(context.Background(), wsID, "owner-1", "Sales", nil)
	require.NoError(t, err)

	list, err := appws.NewListOrgUnits(members, units).Execute(context.Background(), wsID, "owner-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "Sales", list[0].Name)
}

func TestCreateOrgUnitRejectsNonManager(t *testing.T) {
	_, members, units, _, wsID := setupWS(t)
	_ = members.Create(context.Background(), &domain.Member{ID: "m2", WorkspaceID: wsID, UserID: "member-2", Role: domain.RoleMember, Status: domain.StatusActive})
	_, err := appws.NewCreateOrgUnit(members, units).Execute(context.Background(), wsID, "member-2", "X", nil)
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestListOrgUnitsRequiresMembership(t *testing.T) {
	_, members, units, _, wsID := setupWS(t)
	_, err := appws.NewListOrgUnits(members, units).Execute(context.Background(), wsID, "stranger")
	require.ErrorIs(t, err, domain.ErrNotMember)
}

func TestCreateAndListPositions(t *testing.T) {
	_, members, _, positions, wsID := setupWS(t)
	_, err := appws.NewCreatePosition(members, positions).Execute(context.Background(), wsID, "owner-1", "Manager")
	require.NoError(t, err)
	list, err := appws.NewListPositions(members, positions).Execute(context.Background(), wsID, "owner-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestCreatePositionRejectsEmptyTitle(t *testing.T) {
	_, members, _, positions, wsID := setupWS(t)
	_, err := appws.NewCreatePosition(members, positions).Execute(context.Background(), wsID, "owner-1", "  ")
	require.ErrorIs(t, err, domain.ErrInvalidTitle)
}

func TestAssignMember(t *testing.T) {
	_, members, units, positions, wsID := setupWS(t)
	unit, _ := appws.NewCreateOrgUnit(members, units).Execute(context.Background(), wsID, "owner-1", "Sales", nil)
	pos, _ := appws.NewCreatePosition(members, positions).Execute(context.Background(), wsID, "owner-1", "Manager")

	err := appws.NewAssignMember(members, units, positions).Execute(context.Background(), wsID, "owner-1", "owner-1", &unit.ID, &pos.ID)
	require.NoError(t, err)
	m, _ := members.Find(context.Background(), wsID, "owner-1")
	require.NotNil(t, m.OrgUnitID)
	require.Equal(t, unit.ID, *m.OrgUnitID)
}

func TestAssignMemberRejectsForeignUnit(t *testing.T) {
	_, members, units, positions, wsID := setupWS(t)
	foreign := "eeeeeeee-0000-0000-0000-000000000001"
	err := appws.NewAssignMember(members, units, positions).Execute(context.Background(), wsID, "owner-1", "owner-1", &foreign, nil)
	require.ErrorIs(t, err, domain.ErrOrgUnitNotFound)
}

func TestAssignMemberRejectsNonManager(t *testing.T) {
	_, members, units, positions, wsID := setupWS(t)
	_ = members.Create(context.Background(), &domain.Member{ID: "m2", WorkspaceID: wsID, UserID: "member-2", Role: domain.RoleMember, Status: domain.StatusActive})
	err := appws.NewAssignMember(members, units, positions).Execute(context.Background(), wsID, "member-2", "member-2", nil, nil)
	require.ErrorIs(t, err, domain.ErrForbidden)
}
```

- [ ] **Step 3: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/application/workspace/ -run "OrgUnit|Position|Assign" -v` → FAIL.

- [ ] **Step 4: Реализовать** — `internal/application/workspace/org_structure.go`:
```go
package workspace

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

// helper: require requester is an active owner/admin of the workspace.
func requireManager(ctx context.Context, members domain.MemberRepository, wsID, userID string) error {
	m, err := members.Find(ctx, wsID, userID)
	if err != nil {
		return err // ErrNotMember
	}
	if !domain.CanManageMembers(m.Role) {
		return domain.ErrForbidden
	}
	return nil
}

// helper: require requester is a member.
func requireMember(ctx context.Context, members domain.MemberRepository, wsID, userID string) error {
	if _, err := members.Find(ctx, wsID, userID); err != nil {
		return err
	}
	return nil
}

type CreateOrgUnit struct {
	members domain.MemberRepository
	units   domain.OrgUnitRepository
}

func NewCreateOrgUnit(m domain.MemberRepository, u domain.OrgUnitRepository) *CreateOrgUnit {
	return &CreateOrgUnit{members: m, units: u}
}

func (uc *CreateOrgUnit) Execute(ctx context.Context, wsID, requesterID, name string, parentID *string) (*domain.OrgUnit, error) {
	if err := requireManager(ctx, uc.members, wsID, requesterID); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.ErrInvalidName
	}
	if parentID != nil {
		ok, err := uc.units.Exists(ctx, wsID, *parentID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, domain.ErrOrgUnitNotFound
		}
	}
	u := &domain.OrgUnit{ID: uuid.NewString(), WorkspaceID: wsID, ParentID: parentID, Name: name, CreatedAt: time.Now().UTC()}
	if err := uc.units.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

type ListOrgUnits struct {
	members domain.MemberRepository
	units   domain.OrgUnitRepository
}

func NewListOrgUnits(m domain.MemberRepository, u domain.OrgUnitRepository) *ListOrgUnits {
	return &ListOrgUnits{members: m, units: u}
}

func (uc *ListOrgUnits) Execute(ctx context.Context, wsID, requesterID string) ([]*domain.OrgUnit, error) {
	if err := requireMember(ctx, uc.members, wsID, requesterID); err != nil {
		return nil, err
	}
	return uc.units.ListByWorkspace(ctx, wsID)
}

type CreatePosition struct {
	members   domain.MemberRepository
	positions domain.PositionRepository
}

func NewCreatePosition(m domain.MemberRepository, p domain.PositionRepository) *CreatePosition {
	return &CreatePosition{members: m, positions: p}
}

func (uc *CreatePosition) Execute(ctx context.Context, wsID, requesterID, title string) (*domain.Position, error) {
	if err := requireManager(ctx, uc.members, wsID, requesterID); err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, domain.ErrInvalidTitle
	}
	p := &domain.Position{ID: uuid.NewString(), WorkspaceID: wsID, Title: title, CreatedAt: time.Now().UTC()}
	if err := uc.positions.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

type ListPositions struct {
	members   domain.MemberRepository
	positions domain.PositionRepository
}

func NewListPositions(m domain.MemberRepository, p domain.PositionRepository) *ListPositions {
	return &ListPositions{members: m, positions: p}
}

func (uc *ListPositions) Execute(ctx context.Context, wsID, requesterID string) ([]*domain.Position, error) {
	if err := requireMember(ctx, uc.members, wsID, requesterID); err != nil {
		return nil, err
	}
	return uc.positions.ListByWorkspace(ctx, wsID)
}

type AssignMember struct {
	members   domain.MemberRepository
	units     domain.OrgUnitRepository
	positions domain.PositionRepository
}

func NewAssignMember(m domain.MemberRepository, u domain.OrgUnitRepository, p domain.PositionRepository) *AssignMember {
	return &AssignMember{members: m, units: u, positions: p}
}

func (uc *AssignMember) Execute(ctx context.Context, wsID, requesterID, targetUserID string, orgUnitID, positionID *string) error {
	if err := requireManager(ctx, uc.members, wsID, requesterID); err != nil {
		return err
	}
	if _, err := uc.members.Find(ctx, wsID, targetUserID); err != nil {
		return err // target must be a member
	}
	if orgUnitID != nil {
		ok, err := uc.units.Exists(ctx, wsID, *orgUnitID)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrOrgUnitNotFound
		}
	}
	if positionID != nil {
		ok, err := uc.positions.Exists(ctx, wsID, *positionID)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrPositionNotFound
		}
	}
	return uc.members.Assign(ctx, wsID, targetUserID, orgUnitID, positionID)
}
```

- [ ] **Step 5: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/application/workspace/ -v` → PASS (все, включая W1).

- [ ] **Step 6: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/application/workspace/
git commit -m "feat(workspace): org-structure use-cases"
```

---

## Task 5: REST API (org-units/positions/assignment) + DI

**Files:** `internal/presentation/http/workspace_handlers.go` (+ test, + fakes), `internal/infrastructure/di/wire.go`, `internal/infrastructure/httpserver/server.go`.

**Роуты (RequireAuth, current user):**
- `GET /workspaces/{id}/org-units` (member) · `POST /workspaces/{id}/org-units` {name, parent_id?} (owner/admin)
- `GET /workspaces/{id}/positions` (member) · `POST /workspaces/{id}/positions` {title} (owner/admin)
- `PUT /workspaces/{id}/members/{userId}/assignment` {org_unit_id?, position_id?} (owner/admin)

- [ ] **Step 1: Расширить WorkspaceHandlers** — конструктор `NewWorkspaceHandlers` получает доп. use-cases (`*CreateOrgUnit, *ListOrgUnits, *CreatePosition, *ListPositions, *AssignMember`). Добавить методы + маршруты в `Register`. Тело: парсить JSON, брать `UserIDFromContext`, вызывать use-case, `wsErr` для ошибок (добавить кейсы `ErrOrgUnitNotFound`/`ErrPositionNotFound` → 404, `ErrInvalidTitle` → 400). Assignment: nil-указатели, если поле не задано/пустое.
```go
// добавить в Register:
	r.Get("/workspaces/{id}/org-units", h.listOrgUnits)
	r.Post("/workspaces/{id}/org-units", h.createOrgUnit)
	r.Get("/workspaces/{id}/positions", h.listPositions)
	r.Post("/workspaces/{id}/positions", h.createPosition)
	r.Put("/workspaces/{id}/members/{userId}/assignment", h.assignMember)
```
Пример метода:
```go
func (h *WorkspaceHandlers) createOrgUnit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	u, err := h.createOrgUnitUC.Execute(r.Context(), chi.URLParam(r, "id"), UserIDFromContext(r.Context()), body.Name, body.ParentID)
	if err != nil {
		wsErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": u.ID, "name": u.Name, "parent_id": u.ParentID})
}

func (h *WorkspaceHandlers) assignMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrgUnitID  *string `json:"org_unit_id"`
		PositionID *string `json:"position_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	err := h.assignUC.Execute(r.Context(), chi.URLParam(r, "id"), UserIDFromContext(r.Context()), chi.URLParam(r, "userId"), body.OrgUnitID, body.PositionID)
	if err != nil {
		wsErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```
(listOrgUnits/createPosition/listPositions — по аналогии; list-методы отдают JSON-массив DTO. Добавить поля-use-cases в struct `WorkspaceHandlers` и в `NewWorkspaceHandlers`.)
Расширить `wsErr` кейсами:
```go
	case errors.Is(err, domainws.ErrOrgUnitNotFound), errors.Is(err, domainws.ErrPositionNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, domainws.ErrInvalidTitle):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid title"})
```

- [ ] **Step 2: Тесты хендлеров** — добавить в `workspace_handlers_test.go` кейсы: создать+список org-units (как owner), 403 для не-менеджера на POST org-units, assignment ok. Обновить `buildWSAPI` под новую сигнатуру `NewWorkspaceHandlers` (передать новые use-cases, построенные на `fakeOrgUnitsHTTP`/`fakePositionsHTTP` — добавить их в `workspace_fakes_test.go` по образцу app-фейков). Также добавить метод `Assign` к `fakeMembersHTTP`.
Минимум:
```go
func TestCreateAndListOrgUnitsEndpoint(t *testing.T) { /* POST /workspaces/{id}/org-units как owner → 201; GET → 200 с 1 элементом */ }
func TestCreateOrgUnitForbiddenForNonManager(t *testing.T) { /* member → 403 */ }
```
(Реализуй полноценно, следуя стилю существующих workspace-тестов; используй Bearer-токен с subject = owner.)

- [ ] **Step 3: TDD run** — падает → реализовать → проходит. Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -v`.

- [ ] **Step 4: DI** — в `wire.go`: провайдеры `provideOrgUnitRepo`/`providePositionRepo` (→ domainws интерфейсы через `pgc.NewOrgUnitRepository`/`NewPositionRepository`); обновить `provideWorkspaceHandlers`, чтобы строил и передавал новые use-cases (нужны member/orgunit/position репозитории). `make wire && go build ./... && go test -short ./...`.

- [ ] **Step 5: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/presentation/http/ internal/infrastructure/di/
git commit -m "feat(workspace): org-structure REST API + wiring"
```

---

## Task 6: Финальная проверка + push
- [ ] **Step 1:** `cd /Users/denisurevic/Downloads/ББД/platform && go test ./... && go vet ./... && go build ./...` → зелёно/чисто.
- [ ] **Step 2:** `git push origin main`.

---

## Definition of Done (W2)
- Миграция org_units/positions + колонки назначения у members.
- Репо + use-cases: создать/список подразделений (дерево через parent_id) и должностей (мутации — owner/admin, чтение — участник), назначить участника на подразделение+должность (с проверкой принадлежности воркспейсу).
- REST под Bearer-auth; авторизация проверена тестами.
- Все тесты зелёные; vet/build чисто; запушено.

## Следующая фаза
W3: продукты (реестр продуктов + включение в воркспейсе) + REST для продуктов (`/me/workspaces`, `/workspaces/:id/members|org-units|positions` — часть уже есть; добавить product-enablement). W4: UI-свитчер и управление в хабе.
