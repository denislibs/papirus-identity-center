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
