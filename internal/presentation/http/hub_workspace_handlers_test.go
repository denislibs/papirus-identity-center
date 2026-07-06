package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	appws "github.com/denislibs/papirus-identity-center/internal/application/workspace"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

// buildHubWS wires HubWorkspaceHandlers with the workspace http-fakes behind RequireHubSession(user).
func buildHubWS(t *testing.T, userID string) (*httptest.Server, *fakeMembersHTTP) {
	t.Helper()
	members := newFakeMembersHTTP()
	ws := newFakeWSHTTP(members)
	invites := newFakeInvitesHTTP()
	units := &fakeOrgUnitsHTTP{}
	positions := &fakePositionsHTTP{}
	products := &fakeProductsHTTP{list: nil}
	wprods := newFakeWorkspaceProductsHTTP()
	mailer := &fakeWSMailer{}
	store := &fakeHubStore{created: userID, id: "hubid-ws"}

	h := apphttp.NewHubWorkspaceHandlers(
		appws.NewCreateWorkspace(ws, members),
		appws.NewListMyWorkspaces(ws),
		appws.NewListMembers(members),
		appws.NewInviteMember(ws, members, invites, mailer, "https://acc.example"),
		appws.NewCreateOrgUnit(members, units),
		appws.NewListOrgUnits(members, units),
		appws.NewCreatePosition(members, positions),
		appws.NewListPositions(members, positions),
		appws.NewListProducts(products),
		appws.NewEnableProduct(members, products, wprods),
		appws.NewListEnabledProducts(members, wprods),
		apphttp.MustLoadTemplates(),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) { pr.Use(apphttp.RequireHubSession(store)); h.Register(pr) })
	return httptest.NewServer(r), members
}

func noRedir() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func hubReq(t *testing.T, method, url string, body string) *http.Request {
	t.Helper()
	var r *http.Request
	if body == "" {
		r, _ = http.NewRequest(method, url, nil)
	} else {
		r, _ = http.NewRequest(method, url, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	r.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-ws"})
	return r
}

func TestHubWorkspacesListAndCreate(t *testing.T) {
	srv, _ := buildHubWS(t, "user-1")
	defer srv.Close()

	// initially empty list renders
	resp, err := noRedir().Do(hubReq(t, http.MethodGet, srv.URL+"/account/workspaces", ""))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// create → 303 to detail
	form := url.Values{"name": {"Acme"}}.Encode()
	resp2, err := noRedir().Do(hubReq(t, http.MethodPost, srv.URL+"/account/workspaces", form))
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp2.StatusCode)
	require.True(t, strings.HasPrefix(resp2.Header.Get("Location"), "/account/workspaces/"))
	resp2.Body.Close()

	// list now shows Acme
	resp3, err := noRedir().Do(hubReq(t, http.MethodGet, srv.URL+"/account/workspaces", ""))
	require.NoError(t, err)
	b := make([]byte, 8192)
	n, _ := resp3.Body.Read(b)
	resp3.Body.Close()
	require.Contains(t, string(b[:n]), "Acme")
}

func TestHubWorkspaceDetailAndManage(t *testing.T) {
	srv, _ := buildHubWS(t, "owner-x")
	defer srv.Close()

	// create a workspace (owner-x becomes owner)
	form := url.Values{"name": {"Detail Co"}}.Encode()
	resp, err := noRedir().Do(hubReq(t, http.MethodPost, srv.URL+"/account/workspaces", form))
	require.NoError(t, err)
	loc := resp.Header.Get("Location")
	resp.Body.Close()
	require.True(t, strings.HasPrefix(loc, "/account/workspaces/"))

	// detail renders (owner is a member)
	resp2, err := noRedir().Do(hubReq(t, http.MethodGet, srv.URL+loc, ""))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	b := make([]byte, 16384)
	n, _ := resp2.Body.Read(b)
	resp2.Body.Close()
	require.Contains(t, string(b[:n]), "Members")

	// add a department → 303 back
	dept := url.Values{"name": {"Sales"}}.Encode()
	resp3, err := noRedir().Do(hubReq(t, http.MethodPost, srv.URL+loc+"/org-units", dept))
	require.NoError(t, err)
	require.Equal(t, http.StatusSeeOther, resp3.StatusCode)
	resp3.Body.Close()

	// detail now shows Sales
	resp4, err := noRedir().Do(hubReq(t, http.MethodGet, srv.URL+loc, ""))
	require.NoError(t, err)
	b2 := make([]byte, 16384)
	n2, _ := resp4.Body.Read(b2)
	resp4.Body.Close()
	require.Contains(t, string(b2[:n2]), "Sales")
}

var _ = context.Background
