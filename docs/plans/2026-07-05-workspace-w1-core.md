# Workspace модуль — W1 (ядро: воркспейсы, участники, приглашения) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Платформенный слой воркспейсов: пользователь создаёт воркспейс (становится owner), приглашает людей по email, участники видят команду — через аутентифицированный Bearer API.

**Architecture:** Новый ограниченный контекст `workspace` рядом с `identity` (чистая архитектура: domain/application/infrastructure/presentation). Воркспейс — общий (name/slug, без продуктовой специфики). Роли: owner/admin/member. Приглашения по email (переиспользуем Mailer). API под `RequireAuth` (Bearer-introspection из 2b-ii), текущий пользователь = subject.

**Scope note (W1):** воркспейсы + участники + приглашения + REST API. НЕ входит: оргструктура (→ W2), продукты (→ W3), UI-свитчер в хабе (→ W4).

**Tech Stack:** Go 1.26, chi, pgx, golang-migrate (embed), html — нет; testify, testcontainers. Module `github.com/denislibs/papirus-identity-center`.

---

## Предпосылки
Готово: Postgres (`Connect`, `RunMigrations` с embed-миграциями в `internal/infrastructure/postgres/migrations/`, `users` table), `RequireAuth`/`UserIDFromContext` (Bearer introspection), Mailer порт (`SendVerification`/`SendPasswordReset` + Log/SMTP impls), config (`BaseURL`), DI (wire), chi router. `writeJSON` helper в `presentation/http`.

---

## File Structure
```
internal/
  domain/workspace/
    workspace.go       Workspace, WorkspaceMember, WorkspaceInvite + roles/errors
    ports.go           WorkspaceRepository, MemberRepository, InviteRepository, WorkspaceMailer
  application/workspace/
    create_workspace.go     + test
    list_my_workspaces.go   + test
    invite_member.go        + test
    accept_invite.go        + test
    list_members.go         + test
    fakes_test.go           общие фейки для пакета
  infrastructure/
    postgres/migrations/0003_workspaces.up.sql / .down.sql
    postgres/workspace_repository.go   (+ member + invite repos) + test
    mail/log_mailer.go / smtp_mailer.go  (+ SendWorkspaceInvite)
  presentation/http/
    workspace_handlers.go   + test
  infrastructure/di/wire.go (+ провайдеры/mount)
  infrastructure/httpserver/server.go (+ mount под RequireAuth)
```

---

## Task 1: Миграция workspaces/members/invites

**Files:**
- Create: `internal/infrastructure/postgres/migrations/0003_workspaces.up.sql`
- Create: `internal/infrastructure/postgres/migrations/0003_workspaces.down.sql`

- [ ] **Step 1: SQL** — `0003_workspaces.up.sql`:
```sql
CREATE TABLE workspaces (
    id         UUID PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL UNIQUE,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE workspace_members (
    id           UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'active',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, user_id)
);
CREATE INDEX idx_members_user ON workspace_members (user_id);

CREATE TABLE workspace_invites (
    id           UUID PRIMARY KEY,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    email        TEXT NOT NULL,
    role         TEXT NOT NULL,
    token        TEXT NOT NULL UNIQUE,
    expires_at   TIMESTAMPTZ NOT NULL,
    accepted_at  TIMESTAMPTZ
);
CREATE INDEX idx_invites_token ON workspace_invites (token);
```
`0003_workspaces.down.sql`:
```sql
DROP TABLE workspace_invites;
DROP TABLE workspace_members;
DROP TABLE workspaces;
```

- [ ] **Step 2: Тест миграции** — добавить в `internal/infrastructure/postgres/migrate_test.go`:
```go
func TestRunMigrationsCreatesWorkspaces(t *testing.T) {
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
	for _, tbl := range []string{"workspaces", "workspace_members", "workspace_invites"} {
		var exists bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name=$1)`, tbl).Scan(&exists))
		require.True(t, exists, tbl)
	}
}
```

- [ ] **Step 3: Запустить — проходит** (embed подхватывает). Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/infrastructure/postgres/ -run TestRunMigrationsCreatesWorkspaces -v` → PASS.

- [ ] **Step 4: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/postgres/migrations/ internal/infrastructure/postgres/migrate_test.go
git commit -m "feat(workspace): workspaces/members/invites migration"
```

---

## Task 2: Домен workspace (сущности + порты)

**Files:**
- Create: `internal/domain/workspace/workspace.go`
- Create: `internal/domain/workspace/ports.go`

- [ ] **Step 1: Сущности + роли + ошибки** — `internal/domain/workspace/workspace.go`:
```go
package workspace

import (
	"errors"
	"time"
)

// Roles within a workspace (generic, platform-level).
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Member statuses.
const (
	StatusActive  = "active"
	StatusInvited = "invited"
)

type Workspace struct {
	ID        string
	Name      string
	Slug      string
	CreatedBy string
	CreatedAt time.Time
}

type Member struct {
	ID          string
	WorkspaceID string
	UserID      string
	Role        string
	Status      string
	CreatedAt   time.Time
}

type Invite struct {
	ID          string
	WorkspaceID string
	Email       string
	Role        string
	Token       string
	ExpiresAt   time.Time
	AcceptedAt  *time.Time
}

var (
	ErrWorkspaceNotFound = errors.New("workspace: not found")
	ErrInviteNotFound    = errors.New("workspace: invite not found or expired")
	ErrNotMember         = errors.New("workspace: user is not a member")
	ErrForbidden         = errors.New("workspace: insufficient role")
	ErrInvalidName       = errors.New("workspace: invalid name")
	ErrInvalidRole       = errors.New("workspace: invalid role")
)

// CanManageMembers reports whether a role may invite/manage members.
func CanManageMembers(role string) bool { return role == RoleOwner || role == RoleAdmin }

// ValidRole reports whether role is one of the known roles.
func ValidRole(role string) bool {
	return role == RoleOwner || role == RoleAdmin || role == RoleMember
}
```

- [ ] **Step 2: Порты** — `internal/domain/workspace/ports.go`:
```go
package workspace

import "context"

type WorkspaceRepository interface {
	Create(ctx context.Context, w *Workspace) error
	FindByID(ctx context.Context, id string) (*Workspace, error) // ErrWorkspaceNotFound
	ListByMember(ctx context.Context, userID string) ([]*Workspace, error)
	SlugExists(ctx context.Context, slug string) (bool, error)
}

type MemberRepository interface {
	Create(ctx context.Context, m *Member) error
	Find(ctx context.Context, workspaceID, userID string) (*Member, error) // ErrNotMember
	ListByWorkspace(ctx context.Context, workspaceID string) ([]*Member, error)
}

type InviteRepository interface {
	Create(ctx context.Context, inv *Invite) error
	FindByToken(ctx context.Context, token string) (*Invite, error) // ErrInviteNotFound
	MarkAccepted(ctx context.Context, id string) error
}

// WorkspaceMailer sends workspace invitation emails.
type WorkspaceMailer interface {
	SendWorkspaceInvite(ctx context.Context, to, workspaceName, link string) error
}
```

- [ ] **Step 3: Сборка.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go build ./internal/domain/...`

- [ ] **Step 4: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/domain/workspace/
git commit -m "feat(workspace): domain entities + ports"
```

---

## Task 3: Postgres репозитории (workspace + member + invite)

**Files:**
- Create: `internal/infrastructure/postgres/workspace_repository.go`
- Test: `internal/infrastructure/postgres/workspace_repository_test.go`

- [ ] **Step 1: Падающий интеграционный тест** — `internal/infrastructure/postgres/workspace_repository_test.go`:
```go
package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/denislibs/papirus-identity-center/internal/domain/identity"
	"github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

func TestWorkspaceRepositories(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	userRepo := NewUserRepository(w.pool)
	wsRepo := NewWorkspaceRepository(w.pool)
	memRepo := NewMemberRepository(w.pool)
	invRepo := NewInviteRepository(w.pool)

	uid := "55555555-5555-5555-5555-555555555555"
	require.NoError(t, userRepo.Create(ctx, &identity.User{ID: uid, Email: "o@x.com", PasswordHash: "h", Locale: "en", Timezone: "UTC", CreatedAt: time.Now().UTC()}))

	ws := &workspace.Workspace{ID: "66666666-6666-6666-6666-666666666666", Name: "Acme", Slug: "acme", CreatedBy: uid, CreatedAt: time.Now().UTC()}
	require.NoError(t, wsRepo.Create(ctx, ws))

	exists, err := wsRepo.SlugExists(ctx, "acme")
	require.NoError(t, err)
	require.True(t, exists)

	got, err := wsRepo.FindByID(ctx, ws.ID)
	require.NoError(t, err)
	require.Equal(t, "Acme", got.Name)

	owner := &workspace.Member{ID: "77777777-7777-7777-7777-777777777777", WorkspaceID: ws.ID, UserID: uid, Role: workspace.RoleOwner, Status: workspace.StatusActive, CreatedAt: time.Now().UTC()}
	require.NoError(t, memRepo.Create(ctx, owner))

	m, err := memRepo.Find(ctx, ws.ID, uid)
	require.NoError(t, err)
	require.Equal(t, workspace.RoleOwner, m.Role)

	list, err := wsRepo.ListByMember(ctx, uid)
	require.NoError(t, err)
	require.Len(t, list, 1)

	members, err := memRepo.ListByWorkspace(ctx, ws.ID)
	require.NoError(t, err)
	require.Len(t, members, 1)

	_, err = memRepo.Find(ctx, ws.ID, "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, workspace.ErrNotMember)

	inv := &workspace.Invite{ID: "88888888-8888-8888-8888-888888888888", WorkspaceID: ws.ID, Email: "invitee@x.com", Role: workspace.RoleMember, Token: "tok-123", ExpiresAt: time.Now().Add(time.Hour).UTC()}
	require.NoError(t, invRepo.Create(ctx, inv))
	gotInv, err := invRepo.FindByToken(ctx, "tok-123")
	require.NoError(t, err)
	require.Equal(t, ws.ID, gotInv.WorkspaceID)
	require.NoError(t, invRepo.MarkAccepted(ctx, inv.ID))

	_, err = invRepo.FindByToken(ctx, "missing")
	require.ErrorIs(t, err, workspace.ErrInviteNotFound)
}
```

- [ ] **Step 2: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/infrastructure/postgres/ -run TestWorkspaceRepositories -v` → FAIL (нет конструкторов).

- [ ] **Step 3: Реализовать** — `internal/infrastructure/postgres/workspace_repository.go`:
```go
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type WorkspaceRepository struct{ pool *pgxpool.Pool }

func NewWorkspaceRepository(pool *pgxpool.Pool) *WorkspaceRepository { return &WorkspaceRepository{pool} }

func (r *WorkspaceRepository) Create(ctx context.Context, w *workspace.Workspace) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO workspaces (id, name, slug, created_by, created_at) VALUES ($1,$2,$3,$4,$5)`,
		w.ID, w.Name, w.Slug, w.CreatedBy, w.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create workspace: %w", err)
	}
	return nil
}

func (r *WorkspaceRepository) FindByID(ctx context.Context, id string) (*workspace.Workspace, error) {
	var w workspace.Workspace
	err := r.pool.QueryRow(ctx, `SELECT id, name, slug, created_by, created_at FROM workspaces WHERE id=$1`, id).
		Scan(&w.ID, &w.Name, &w.Slug, &w.CreatedBy, &w.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, workspace.ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find workspace: %w", err)
	}
	return &w, nil
}

func (r *WorkspaceRepository) ListByMember(ctx context.Context, userID string) ([]*workspace.Workspace, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT w.id, w.name, w.slug, w.created_by, w.created_at
		 FROM workspaces w JOIN workspace_members m ON m.workspace_id = w.id
		 WHERE m.user_id=$1 AND m.status='active' ORDER BY w.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list workspaces: %w", err)
	}
	defer rows.Close()
	var out []*workspace.Workspace
	for rows.Next() {
		var w workspace.Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Slug, &w.CreatedBy, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan workspace: %w", err)
		}
		out = append(out, &w)
	}
	return out, rows.Err()
}

func (r *WorkspaceRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspaces WHERE slug=$1)`, slug).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("postgres: slug exists: %w", err)
	}
	return exists, nil
}

type MemberRepository struct{ pool *pgxpool.Pool }

func NewMemberRepository(pool *pgxpool.Pool) *MemberRepository { return &MemberRepository{pool} }

func (r *MemberRepository) Create(ctx context.Context, m *workspace.Member) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO workspace_members (id, workspace_id, user_id, role, status, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		m.ID, m.WorkspaceID, m.UserID, m.Role, m.Status, m.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create member: %w", err)
	}
	return nil
}

func (r *MemberRepository) Find(ctx context.Context, workspaceID, userID string) (*workspace.Member, error) {
	var m workspace.Member
	err := r.pool.QueryRow(ctx, `SELECT id, workspace_id, user_id, role, status, created_at FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, userID).
		Scan(&m.ID, &m.WorkspaceID, &m.UserID, &m.Role, &m.Status, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, workspace.ErrNotMember
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find member: %w", err)
	}
	return &m, nil
}

func (r *MemberRepository) ListByWorkspace(ctx context.Context, workspaceID string) ([]*workspace.Member, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, workspace_id, user_id, role, status, created_at FROM workspace_members WHERE workspace_id=$1 ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list members: %w", err)
	}
	defer rows.Close()
	var out []*workspace.Member
	for rows.Next() {
		var m workspace.Member
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.UserID, &m.Role, &m.Status, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan member: %w", err)
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

type InviteRepository struct{ pool *pgxpool.Pool }

func NewInviteRepository(pool *pgxpool.Pool) *InviteRepository { return &InviteRepository{pool} }

func (r *InviteRepository) Create(ctx context.Context, inv *workspace.Invite) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO workspace_invites (id, workspace_id, email, role, token, expires_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		inv.ID, inv.WorkspaceID, inv.Email, inv.Role, inv.Token, inv.ExpiresAt)
	if err != nil {
		return fmt.Errorf("postgres: create invite: %w", err)
	}
	return nil
}

func (r *InviteRepository) FindByToken(ctx context.Context, token string) (*workspace.Invite, error) {
	var inv workspace.Invite
	err := r.pool.QueryRow(ctx, `SELECT id, workspace_id, email, role, token, expires_at, accepted_at FROM workspace_invites WHERE token=$1 AND accepted_at IS NULL AND expires_at > now()`, token).
		Scan(&inv.ID, &inv.WorkspaceID, &inv.Email, &inv.Role, &inv.Token, &inv.ExpiresAt, &inv.AcceptedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, workspace.ErrInviteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find invite: %w", err)
	}
	return &inv, nil
}

func (r *InviteRepository) MarkAccepted(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE workspace_invites SET accepted_at=now() WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("postgres: mark invite accepted: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/infrastructure/postgres/ -run TestWorkspaceRepositories -v` → PASS.

- [ ] **Step 5: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/postgres/workspace_repository.go internal/infrastructure/postgres/workspace_repository_test.go
git commit -m "feat(workspace): postgres repositories"
```

---

## Task 4: Mailer — SendWorkspaceInvite

**Files:**
- Modify: `internal/infrastructure/mail/log_mailer.go`, `smtp_mailer.go`

- [ ] **Step 1: Добавить метод** — в `log_mailer.go`:
```go
func (m *LogMailer) SendWorkspaceInvite(_ context.Context, to, workspaceName, link string) error {
	m.logger.Printf("[mail] workspace-invite to=%s workspace=%q link=%s", to, workspaceName, link)
	return nil
}
```
в `smtp_mailer.go`:
```go
func (m *SMTPMailer) SendWorkspaceInvite(_ context.Context, to, workspaceName, link string) error {
	return m.send(to, "You're invited to "+workspaceName, "Join "+workspaceName+": "+link)
}
```
(both types now satisfy both `identity.Mailer` and `workspace.WorkspaceMailer`.)

- [ ] **Step 2: Сборка.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go build ./internal/infrastructure/mail/`

- [ ] **Step 3: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/mail/
git commit -m "feat(workspace): SendWorkspaceInvite on mailers"
```

---

## Task 5: Use-cases (Create/List/Invite/Accept/ListMembers)

**Files:**
- Create: `internal/application/workspace/{create_workspace,list_my_workspaces,invite_member,accept_invite,list_members}.go`
- Test: `internal/application/workspace/*_test.go` + `fakes_test.go`

- [ ] **Step 1: Фейки** — `internal/application/workspace/fakes_test.go`:
```go
package workspace_test

import (
	"context"
	"strings"
	"time"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type fakeWS struct{ byID map[string]*domain.Workspace; slugs map[string]bool; members *fakeMembers }

func newFakeWS(m *fakeMembers) *fakeWS {
	return &fakeWS{byID: map[string]*domain.Workspace{}, slugs: map[string]bool{}, members: m}
}
func (f *fakeWS) Create(_ context.Context, w *domain.Workspace) error {
	cp := *w; f.byID[w.ID] = &cp; f.slugs[w.Slug] = true; return nil
}
func (f *fakeWS) FindByID(_ context.Context, id string) (*domain.Workspace, error) {
	if w, ok := f.byID[id]; ok { cp := *w; return &cp, nil }
	return nil, domain.ErrWorkspaceNotFound
}
func (f *fakeWS) ListByMember(_ context.Context, userID string) ([]*domain.Workspace, error) {
	var out []*domain.Workspace
	for _, m := range f.members.list {
		if m.UserID == userID && m.Status == domain.StatusActive {
			if w, ok := f.byID[m.WorkspaceID]; ok { cp := *w; out = append(out, &cp) }
		}
	}
	return out, nil
}
func (f *fakeWS) SlugExists(_ context.Context, slug string) (bool, error) { return f.slugs[slug], nil }

type fakeMembers struct{ list []*domain.Member }

func newFakeMembers() *fakeMembers { return &fakeMembers{} }
func (f *fakeMembers) Create(_ context.Context, m *domain.Member) error { cp := *m; f.list = append(f.list, &cp); return nil }
func (f *fakeMembers) Find(_ context.Context, wsID, userID string) (*domain.Member, error) {
	for _, m := range f.list { if m.WorkspaceID == wsID && m.UserID == userID { cp := *m; return &cp, nil } }
	return nil, domain.ErrNotMember
}
func (f *fakeMembers) ListByWorkspace(_ context.Context, wsID string) ([]*domain.Member, error) {
	var out []*domain.Member
	for _, m := range f.list { if m.WorkspaceID == wsID { cp := *m; out = append(out, &cp) } }
	return out, nil
}

type fakeInvites struct{ byToken map[string]*domain.Invite; accepted map[string]bool }

func newFakeInvites() *fakeInvites { return &fakeInvites{byToken: map[string]*domain.Invite{}, accepted: map[string]bool{}} }
func (f *fakeInvites) Create(_ context.Context, inv *domain.Invite) error { cp := *inv; f.byToken[inv.Token] = &cp; return nil }
func (f *fakeInvites) FindByToken(_ context.Context, token string) (*domain.Invite, error) {
	if inv, ok := f.byToken[token]; ok && !f.accepted[inv.ID] { cp := *inv; return &cp, nil }
	return nil, domain.ErrInviteNotFound
}
func (f *fakeInvites) MarkAccepted(_ context.Context, id string) error { f.accepted[id] = true; return nil }

type sentInvite struct{ to, ws, link string }
type fakeMailer struct{ invites []sentInvite }

func (f *fakeMailer) SendWorkspaceInvite(_ context.Context, to, ws, link string) error {
	f.invites = append(f.invites, sentInvite{to, ws, link}); return nil
}

// slug helper mirror for assertions
func slugContains(s, sub string) bool { return strings.Contains(s, sub) }
var _ = time.Now
```

- [ ] **Step 2: Падающие тесты** — `internal/application/workspace/workspace_test.go`:
```go
package workspace_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	appws "github.com/denislibs/papirus-identity-center/internal/application/workspace"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

func TestCreateWorkspaceMakesOwner(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	uc := appws.NewCreateWorkspace(ws, members)

	w, err := uc.Execute(context.Background(), "user-1", "Acme Inc")
	require.NoError(t, err)
	require.NotEmpty(t, w.ID)
	require.NotEmpty(t, w.Slug)
	require.Equal(t, "user-1", w.CreatedBy)

	// creator is an active owner member
	m, err := members.Find(context.Background(), w.ID, "user-1")
	require.NoError(t, err)
	require.Equal(t, domain.RoleOwner, m.Role)
	require.Equal(t, domain.StatusActive, m.Status)
}

func TestCreateWorkspaceRejectsEmptyName(t *testing.T) {
	members := newFakeMembers()
	uc := appws.NewCreateWorkspace(newFakeWS(members), members)
	_, err := uc.Execute(context.Background(), "user-1", "   ")
	require.ErrorIs(t, err, domain.ErrInvalidName)
}

func TestListMyWorkspaces(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	create := appws.NewCreateWorkspace(ws, members)
	_, _ = create.Execute(context.Background(), "user-1", "A")
	_, _ = create.Execute(context.Background(), "user-1", "B")

	list, err := appws.NewListMyWorkspaces(ws).Execute(context.Background(), "user-1")
	require.NoError(t, err)
	require.Len(t, list, 2)
}

func TestInviteMemberByOwnerSendsEmail(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	invites := newFakeInvites()
	mailer := &fakeMailer{}
	w, _ := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Acme")

	uc := appws.NewInviteMember(ws, members, invites, mailer, "https://acc.example")
	err := uc.Execute(context.Background(), w.ID, "owner-1", "invitee@x.com", domain.RoleMember)
	require.NoError(t, err)
	require.Len(t, mailer.invites, 1)
	require.Equal(t, "invitee@x.com", mailer.invites[0].to)
}

func TestInviteMemberRejectsNonManager(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	w, _ := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Acme")
	// add a plain member
	_ = members.Create(context.Background(), &domain.Member{ID: "m2", WorkspaceID: w.ID, UserID: "member-2", Role: domain.RoleMember, Status: domain.StatusActive})

	uc := appws.NewInviteMember(ws, members, newFakeInvites(), &fakeMailer{}, "https://acc.example")
	err := uc.Execute(context.Background(), w.ID, "member-2", "x@x.com", domain.RoleMember)
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestInviteMemberRejectsInvalidRole(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	w, _ := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Acme")
	uc := appws.NewInviteMember(ws, members, newFakeInvites(), &fakeMailer{}, "https://acc.example")
	err := uc.Execute(context.Background(), w.ID, "owner-1", "x@x.com", "superadmin")
	require.ErrorIs(t, err, domain.ErrInvalidRole)
}

func TestAcceptInviteAddsMember(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	invites := newFakeInvites()
	w, _ := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Acme")
	_ = appws.NewInviteMember(ws, members, invites, &fakeMailer{}, "https://acc.example").
		Execute(context.Background(), w.ID, "owner-1", "invitee@x.com", domain.RoleMember)
	// grab the issued token
	var token string
	for tok := range invites.byToken { token = tok }

	err := appws.NewAcceptInvite(invites, members).Execute(context.Background(), token, "invitee-user")
	require.NoError(t, err)
	m, err := members.Find(context.Background(), w.ID, "invitee-user")
	require.NoError(t, err)
	require.Equal(t, domain.RoleMember, m.Role)
}

func TestAcceptInviteRejectsBadToken(t *testing.T) {
	err := appws.NewAcceptInvite(newFakeInvites(), newFakeMembers()).Execute(context.Background(), "nope", "u")
	require.ErrorIs(t, err, domain.ErrInviteNotFound)
}

func TestListMembersRequiresMembership(t *testing.T) {
	members := newFakeMembers()
	ws := newFakeWS(members)
	w, _ := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Acme")

	uc := appws.NewListMembers(members)
	// owner can list
	list, err := uc.Execute(context.Background(), w.ID, "owner-1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	// non-member cannot
	_, err = uc.Execute(context.Background(), w.ID, "stranger")
	require.ErrorIs(t, err, domain.ErrNotMember)
}
```

- [ ] **Step 3: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/application/workspace/ -v` → FAIL.

- [ ] **Step 4: Реализовать use-cases.**

`create_workspace.go`:
```go
package workspace

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type CreateWorkspace struct {
	workspaces domain.WorkspaceRepository
	members    domain.MemberRepository
}

func NewCreateWorkspace(w domain.WorkspaceRepository, m domain.MemberRepository) *CreateWorkspace {
	return &CreateWorkspace{workspaces: w, members: m}
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "workspace"
	}
	return s
}

func (uc *CreateWorkspace) Execute(ctx context.Context, userID, name string) (*domain.Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.ErrInvalidName
	}
	base := slugify(name)
	slug := base + "-" + uuid.NewString()[:8] // short suffix to avoid collisions
	if exists, err := uc.workspaces.SlugExists(ctx, slug); err != nil {
		return nil, err
	} else if exists {
		slug = base + "-" + uuid.NewString()[:8]
	}
	w := &domain.Workspace{ID: uuid.NewString(), Name: name, Slug: slug, CreatedBy: userID, CreatedAt: time.Now().UTC()}
	if err := uc.workspaces.Create(ctx, w); err != nil {
		return nil, err
	}
	owner := &domain.Member{ID: uuid.NewString(), WorkspaceID: w.ID, UserID: userID, Role: domain.RoleOwner, Status: domain.StatusActive, CreatedAt: time.Now().UTC()}
	if err := uc.members.Create(ctx, owner); err != nil {
		return nil, err
	}
	return w, nil
}
```

`list_my_workspaces.go`:
```go
package workspace

import (
	"context"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type ListMyWorkspaces struct{ workspaces domain.WorkspaceRepository }

func NewListMyWorkspaces(w domain.WorkspaceRepository) *ListMyWorkspaces { return &ListMyWorkspaces{w} }

func (uc *ListMyWorkspaces) Execute(ctx context.Context, userID string) ([]*domain.Workspace, error) {
	return uc.workspaces.ListByMember(ctx, userID)
}
```

`invite_member.go`:
```go
package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

const inviteTTL = 7 * 24 * time.Hour

type InviteMember struct {
	workspaces domain.WorkspaceRepository
	members    domain.MemberRepository
	invites    domain.InviteRepository
	mailer     domain.WorkspaceMailer
	baseURL    string
}

func NewInviteMember(w domain.WorkspaceRepository, m domain.MemberRepository, i domain.InviteRepository,
	mailer domain.WorkspaceMailer, baseURL string) *InviteMember {
	return &InviteMember{workspaces: w, members: m, invites: i, mailer: mailer, baseURL: baseURL}
}

func (uc *InviteMember) Execute(ctx context.Context, workspaceID, inviterID, email, role string) error {
	if !domain.ValidRole(role) {
		return domain.ErrInvalidRole
	}
	inviter, err := uc.members.Find(ctx, workspaceID, inviterID)
	if err != nil {
		return err // ErrNotMember
	}
	if !domain.CanManageMembers(inviter.Role) {
		return domain.ErrForbidden
	}
	ws, err := uc.workspaces.FindByID(ctx, workspaceID)
	if err != nil {
		return err
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("workspace: gen invite token: %w", err)
	}
	token := hex.EncodeToString(buf)
	inv := &domain.Invite{
		ID: uuid.NewString(), WorkspaceID: workspaceID, Email: strings.ToLower(strings.TrimSpace(email)),
		Role: role, Token: token, ExpiresAt: time.Now().Add(inviteTTL).UTC(),
	}
	if err := uc.invites.Create(ctx, inv); err != nil {
		return err
	}
	link := uc.baseURL + "/invites/" + token
	return uc.mailer.SendWorkspaceInvite(ctx, inv.Email, ws.Name, link)
}
```

`accept_invite.go`:
```go
package workspace

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type AcceptInvite struct {
	invites domain.InviteRepository
	members domain.MemberRepository
}

func NewAcceptInvite(i domain.InviteRepository, m domain.MemberRepository) *AcceptInvite {
	return &AcceptInvite{invites: i, members: m}
}

func (uc *AcceptInvite) Execute(ctx context.Context, token, userID string) error {
	inv, err := uc.invites.FindByToken(ctx, token)
	if err != nil {
		return err // ErrInviteNotFound
	}
	m := &domain.Member{ID: uuid.NewString(), WorkspaceID: inv.WorkspaceID, UserID: userID, Role: inv.Role, Status: domain.StatusActive, CreatedAt: time.Now().UTC()}
	if err := uc.members.Create(ctx, m); err != nil {
		return err
	}
	return uc.invites.MarkAccepted(ctx, inv.ID)
}
```

`list_members.go`:
```go
package workspace

import (
	"context"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type ListMembers struct{ members domain.MemberRepository }

func NewListMembers(m domain.MemberRepository) *ListMembers { return &ListMembers{m} }

func (uc *ListMembers) Execute(ctx context.Context, workspaceID, requesterID string) ([]*domain.Member, error) {
	if _, err := uc.members.Find(ctx, workspaceID, requesterID); err != nil {
		return nil, err // ErrNotMember → caller maps to 403/404
	}
	return uc.members.ListByWorkspace(ctx, workspaceID)
}
```

- [ ] **Step 5: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/application/workspace/ -v` → PASS. (`go mod tidy` if needed.)

- [ ] **Step 6: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/application/workspace/ go.mod go.sum
git commit -m "feat(workspace): create/list/invite/accept/list-members use-cases"
```

---

## Task 6: REST API (/workspaces, /me/workspaces, members, invites)

**Files:**
- Create: `internal/presentation/http/workspace_handlers.go`
- Test: `internal/presentation/http/workspace_handlers_test.go`

**Эндпоинты (под RequireAuth; current user = UserIDFromContext):**
- `POST /workspaces` {name} → 201 {id, slug}
- `GET /me/workspaces` → [{id,name,slug}]
- `GET /workspaces/{id}/members` → [{user_id,role,status}] (member-only → 403 ErrNotMember)
- `POST /workspaces/{id}/invites` {email, role} → 202 (owner/admin only → 403)
- `POST /invites/{token}/accept` → 204

- [ ] **Step 1: Падающий тест** — `internal/presentation/http/workspace_handlers_test.go`:
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

	appws "github.com/denislibs/papirus-identity-center/internal/application/workspace"
	domainws "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

func buildWSAPI(t *testing.T, userID string) (*httptest.Server, *fakeHydra) {
	t.Helper()
	members := newFakeMembersHTTP()
	ws := newFakeWSHTTP(members)
	invites := newFakeInvitesHTTP()
	mailer := &fakeWSMailer{}
	hydra := &fakeHydra{introspectActive: true, introspectSubject: userID}
	h := apphttp.NewWorkspaceHandlers(
		appws.NewCreateWorkspace(ws, members),
		appws.NewListMyWorkspaces(ws),
		appws.NewListMembers(members),
		appws.NewInviteMember(ws, members, invites, mailer, "https://acc.example"),
		appws.NewAcceptInvite(invites, members),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) { pr.Use(apphttp.RequireAuth(hydra)); h.Register(pr) })
	return httptest.NewServer(r), hydra
}

func TestCreateAndListWorkspaces(t *testing.T) {
	srv, _ := buildWSAPI(t, "user-1")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"name": "Acme"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created); resp.Body.Close()
	require.NotEmpty(t, created["id"])

	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/me/workspaces", nil)
	req2.Header.Set("Authorization", "Bearer t")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	var list []map[string]any
	json.NewDecoder(resp2.Body).Decode(&list); resp2.Body.Close()
	require.Len(t, list, 1)
}

func TestWorkspaceAPIRequiresAuth(t *testing.T) {
	srv, _ := buildWSAPI(t, "user-1")
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/me/workspaces")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

// helper fakes (HTTP package copies) — see fakes below
var _ = context.Background
var _ = domainws.RoleOwner
```

- [ ] **Step 2: Фейки для http-пакета** — `internal/presentation/http/workspace_fakes_test.go`: copy the in-memory `fakeWS`/`fakeMembers`/`fakeInvites`/`fakeMailer` from the application test package, renamed for this package (`fakeWSHTTP`/`fakeMembersHTTP`/`fakeInvitesHTTP`/`fakeWSMailer`) implementing the `domainws` interfaces. (Same logic as `application/workspace/fakes_test.go`; adapt names + constructors `newFakeWSHTTP`/`newFakeMembersHTTP`/`newFakeInvitesHTTP`.)

- [ ] **Step 3: Запустить — падает.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -run "TestCreateAndListWorkspaces|TestWorkspaceAPIRequiresAuth" -v` → FAIL.

- [ ] **Step 4: Реализовать** — `internal/presentation/http/workspace_handlers.go`:
```go
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	appws "github.com/denislibs/papirus-identity-center/internal/application/workspace"
	domainws "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type WorkspaceHandlers struct {
	create   *appws.CreateWorkspace
	listMine *appws.ListMyWorkspaces
	members  *appws.ListMembers
	invite   *appws.InviteMember
	accept   *appws.AcceptInvite
}

func NewWorkspaceHandlers(create *appws.CreateWorkspace, listMine *appws.ListMyWorkspaces,
	members *appws.ListMembers, invite *appws.InviteMember, accept *appws.AcceptInvite) *WorkspaceHandlers {
	return &WorkspaceHandlers{create: create, listMine: listMine, members: members, invite: invite, accept: accept}
}

func (h *WorkspaceHandlers) Register(r chi.Router) {
	r.Post("/workspaces", h.createWorkspace)
	r.Get("/me/workspaces", h.listMyWorkspaces)
	r.Get("/workspaces/{id}/members", h.listMembers)
	r.Post("/workspaces/{id}/invites", h.inviteMember)
	r.Post("/invites/{token}/accept", h.acceptInvite)
}

func wsErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domainws.ErrInvalidName):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid name"})
	case errors.Is(err, domainws.ErrInvalidRole):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role"})
	case errors.Is(err, domainws.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
	case errors.Is(err, domainws.ErrNotMember):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member"})
	case errors.Is(err, domainws.ErrWorkspaceNotFound), errors.Is(err, domainws.ErrInviteNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (h *WorkspaceHandlers) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct{ Name string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	ws, err := h.create.Execute(r.Context(), UserIDFromContext(r.Context()), body.Name)
	if err != nil {
		wsErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": ws.ID, "slug": ws.Slug})
}

func (h *WorkspaceHandlers) listMyWorkspaces(w http.ResponseWriter, r *http.Request) {
	list, err := h.listMine.Execute(r.Context(), UserIDFromContext(r.Context()))
	if err != nil {
		wsErr(w, err)
		return
	}
	type dto struct{ ID, Name, Slug string }
	out := make([]dto, 0, len(list))
	for _, ws := range list {
		out = append(out, dto{ws.ID, ws.Name, ws.Slug})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *WorkspaceHandlers) listMembers(w http.ResponseWriter, r *http.Request) {
	list, err := h.members.Execute(r.Context(), chi.URLParam(r, "id"), UserIDFromContext(r.Context()))
	if err != nil {
		wsErr(w, err)
		return
	}
	type dto struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
		Status string `json:"status"`
	}
	out := make([]dto, 0, len(list))
	for _, m := range list {
		out = append(out, dto{m.UserID, m.Role, m.Status})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *WorkspaceHandlers) inviteMember(w http.ResponseWriter, r *http.Request) {
	var body struct{ Email, Role string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	err := h.invite.Execute(r.Context(), chi.URLParam(r, "id"), UserIDFromContext(r.Context()), body.Email, body.Role)
	if err != nil {
		wsErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "invited"})
}

func (h *WorkspaceHandlers) acceptInvite(w http.ResponseWriter, r *http.Request) {
	if err := h.accept.Execute(r.Context(), chi.URLParam(r, "token"), UserIDFromContext(r.Context())); err != nil {
		wsErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 5: Запустить — проходит.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && go test ./internal/presentation/http/ -v` → PASS (все).

- [ ] **Step 6: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/presentation/http/workspace_handlers.go internal/presentation/http/workspace_handlers_test.go internal/presentation/http/workspace_fakes_test.go
git commit -m "feat(workspace): REST API (workspaces/members/invites)"
```

---

## Task 7: DI-проводка + mount под RequireAuth

**Files:**
- Modify: `internal/infrastructure/di/wire.go` (+ regen)
- Modify: `internal/infrastructure/httpserver/server.go`

- [ ] **Step 1: Провайдеры** — в `wire.go`:
```go
func provideWorkspaceRepo(pool *pgxpool.Pool) domainws.WorkspaceRepository { return pgc.NewWorkspaceRepository(pool) }
func provideMemberRepo(pool *pgxpool.Pool) domainws.MemberRepository       { return pgc.NewMemberRepository(pool) }
func provideInviteRepo(pool *pgxpool.Pool) domainws.InviteRepository       { return pgc.NewInviteRepository(pool) }
func provideWorkspaceMailer(cfg config.Config) domainws.WorkspaceMailer {
	if cfg.Mail.Mode == "smtp" {
		return mail.NewSMTPMailer(cfg.Mail.Host, cfg.Mail.Port, cfg.Mail.User, cfg.Mail.Password, cfg.Mail.From)
	}
	return mail.NewLogMailer(log.New(os.Stdout, "", log.LstdFlags))
}
func provideWorkspaceHandlers(cfg config.Config, ws domainws.WorkspaceRepository, mem domainws.MemberRepository,
	inv domainws.InviteRepository, mailer domainws.WorkspaceMailer) *apphttp.WorkspaceHandlers {
	return apphttp.NewWorkspaceHandlers(
		appws.NewCreateWorkspace(ws, mem),
		appws.NewListMyWorkspaces(ws),
		appws.NewListMembers(mem),
		appws.NewInviteMember(ws, mem, inv, mailer, cfg.BaseURL),
		appws.NewAcceptInvite(inv, mem),
	)
}
```
Обновить `provideServer` — добавить `wsHandlers *apphttp.WorkspaceHandlers`. Добавить все провайдеры в `wire.Build`; импорты `domainws`, `appws`. (NB: `log`/`os`/`mail` уже импортированы для provideMailer.)

- [ ] **Step 2: Mount** — `server.go`: расширить `NewRouter` параметром `wsHandlers *apphttp.WorkspaceHandlers`; в существующую Bearer-группу (где `sessions.Register(pr)` под `RequireAuth`) добавить `wsHandlers.Register(pr)`. Обновить `server_test.go` вызов `NewRouter` с `apphttp.NewWorkspaceHandlers(nil,nil,nil,nil,nil)` (только `/healthz` тестируется).

- [ ] **Step 3: Regen + build + test.** Run: `cd /Users/denisurevic/Downloads/ББД/platform && make wire && go build ./... && go vet ./... && go test -short ./...` → чисто/зелёно.

- [ ] **Step 4: Commit**
```bash
cd /Users/denisurevic/Downloads/ББД/platform
git add internal/infrastructure/di/ internal/infrastructure/httpserver/
git commit -m "feat(workspace): wire workspace API under auth"
```

---

## Task 8: Финальная проверка + push

- [ ] **Step 1:** `cd /Users/denisurevic/Downloads/ББД/platform && go test ./... && go vet ./... && go build ./...` → всё зелёно/чисто.
- [ ] **Step 2:** `git push origin main`.

---

## Definition of Done (W1)
- Миграция workspaces/members/invites; Postgres репозитории (integration-tested).
- Use-cases: создать воркспейс (создатель → owner), список моих воркспейсов, пригласить (только owner/admin, валидная роль), принять инвайт, список участников (только участник).
- REST API под Bearer-auth: POST /workspaces, GET /me/workspaces, GET /workspaces/{id}/members, POST /workspaces/{id}/invites, POST /invites/{token}/accept.
- Авторизация: не-участник не видит команду; не-owner/admin не может приглашать.
- Все тесты зелёные; vet/build чисто; запушено.

## Следующие фазы
W2: оргструктура (подразделения-дерево + должности + назначение участников) + API. W3: продукты (реестр + включение в воркспейсе) + REST для продуктов. W4: UI в хабе (свитчер воркспейсов, создание, управление участниками/структурой).
