package postgres

import (
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
	require.NoError(t, err)
	require.True(t, ok)

	pos := &workspace.Position{ID: "dddddddd-0000-0000-0000-000000000001", WorkspaceID: wsID, Title: "Manager", CreatedAt: time.Now().UTC()}
	require.NoError(t, posRepo.Create(ctx, pos))
	positions, err := posRepo.ListByWorkspace(ctx, wsID)
	require.NoError(t, err)
	require.Len(t, positions, 1)

	require.NoError(t, memRepo.Assign(ctx, wsID, uid, &unit.ID, &pos.ID))
	m, err := memRepo.Find(ctx, wsID, uid)
	require.NoError(t, err)
	require.NotNil(t, m.OrgUnitID)
	require.Equal(t, unit.ID, *m.OrgUnitID)
	require.NotNil(t, m.PositionID)
	require.Equal(t, pos.ID, *m.PositionID)
}

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
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = prodRepo.Exists(ctx, "nope")
	require.NoError(t, err)
	require.False(t, ok)

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

func TestMemberRepositoryCreateDuplicateReturnsErrAlreadyMember(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	userRepo := NewUserRepository(w.pool)
	wsRepo := NewWorkspaceRepository(w.pool)
	memRepo := NewMemberRepository(w.pool)

	uid := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	require.NoError(t, userRepo.Create(ctx, &identity.User{ID: uid, Email: "dup-member@x.com", PasswordHash: "h", Locale: "en", Timezone: "UTC", CreatedAt: time.Now().UTC()}))

	ws := &workspace.Workspace{ID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Name: "DupTest", Slug: "dup-test", CreatedBy: uid, CreatedAt: time.Now().UTC()}
	require.NoError(t, wsRepo.Create(ctx, ws))

	first := &workspace.Member{ID: "cccccccc-cccc-cccc-cccc-cccccccccccc", WorkspaceID: ws.ID, UserID: uid, Role: workspace.RoleOwner, Status: workspace.StatusActive, CreatedAt: time.Now().UTC()}
	require.NoError(t, memRepo.Create(ctx, first))

	second := &workspace.Member{ID: "dddddddd-dddd-dddd-dddd-dddddddddddd", WorkspaceID: ws.ID, UserID: uid, Role: workspace.RoleMember, Status: workspace.StatusActive, CreatedAt: time.Now().UTC()}
	err := memRepo.Create(ctx, second)
	require.ErrorIs(t, err, workspace.ErrAlreadyMember)
}
