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
