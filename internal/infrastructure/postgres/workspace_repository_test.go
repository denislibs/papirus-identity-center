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
