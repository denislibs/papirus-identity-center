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
