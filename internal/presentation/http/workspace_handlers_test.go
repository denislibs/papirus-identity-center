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
	units := &fakeOrgUnitsHTTP{}
	positions := &fakePositionsHTTP{}
	products := &fakeProductsHTTP{list: []*domainws.Product{
		{Key: "papyrus", Name: "Papyrus (СЭД)"},
		{Key: "lite", Name: "Papyrus Lite"},
	}}
	wp := newFakeWorkspaceProductsHTTP()
	hydra := &fakeHydra{introspectActive: true, introspectSubject: userID}
	h := apphttp.NewWorkspaceHandlers(
		appws.NewCreateWorkspace(ws, members),
		appws.NewListMyWorkspaces(ws),
		appws.NewListMembers(members),
		appws.NewInviteMember(ws, members, invites, mailer, "https://acc.example"),
		appws.NewAcceptInvite(invites, members),
		appws.NewCreateOrgUnit(members, units),
		appws.NewListOrgUnits(members, units),
		appws.NewCreatePosition(members, positions),
		appws.NewListPositions(members, positions),
		appws.NewAssignMember(members, units, positions),
		appws.NewListProducts(products),
		appws.NewEnableProduct(members, products, wp),
		appws.NewDisableProduct(members, wp),
		appws.NewListEnabledProducts(members, wp),
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
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	require.NotEmpty(t, created["id"])

	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/me/workspaces", nil)
	req2.Header.Set("Authorization", "Bearer t")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	var list []map[string]any
	json.NewDecoder(resp2.Body).Decode(&list)
	resp2.Body.Close()
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

func TestListMembers(t *testing.T) {
	srv, _ := buildWSAPI(t, "owner-1")
	defer srv.Close()

	// create workspace first
	body, _ := json.Marshal(map[string]string{"name": "Team"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	wsID := created["id"].(string)

	// owner can list members
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/workspaces/"+wsID+"/members", nil)
	req2.Header.Set("Authorization", "Bearer t")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	var members []map[string]any
	json.NewDecoder(resp2.Body).Decode(&members)
	resp2.Body.Close()
	require.Len(t, members, 1)
	require.Equal(t, "owner-1", members[0]["user_id"])
	require.Equal(t, domainws.RoleOwner, members[0]["role"])
}

func TestListMembersForbiddenForNonMember(t *testing.T) {
	// Create workspace as owner, then try to list as stranger (different server userID)
	members := newFakeMembersHTTP()
	ws := newFakeWSHTTP(members)
	invites := newFakeInvitesHTTP()
	mailer := &fakeWSMailer{}

	// Use a hydra that returns "stranger" as subject
	hydra := &fakeHydra{introspectActive: true, introspectSubject: "stranger"}
	units := &fakeOrgUnitsHTTP{}
	positions := &fakePositionsHTTP{}
	products := &fakeProductsHTTP{}
	wp := newFakeWorkspaceProductsHTTP()
	h := apphttp.NewWorkspaceHandlers(
		appws.NewCreateWorkspace(ws, members),
		appws.NewListMyWorkspaces(ws),
		appws.NewListMembers(members),
		appws.NewInviteMember(ws, members, invites, mailer, "https://acc.example"),
		appws.NewAcceptInvite(invites, members),
		appws.NewCreateOrgUnit(members, units),
		appws.NewListOrgUnits(members, units),
		appws.NewCreatePosition(members, positions),
		appws.NewListPositions(members, positions),
		appws.NewAssignMember(members, units, positions),
		appws.NewListProducts(products),
		appws.NewEnableProduct(members, products, wp),
		appws.NewDisableProduct(members, wp),
		appws.NewListEnabledProducts(members, wp),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) { pr.Use(apphttp.RequireAuth(hydra)); h.Register(pr) })
	srv := httptest.NewServer(r)
	defer srv.Close()

	// Manually pre-seed a workspace for owner-1 (not stranger)
	_ = context.Background()
	wsCreate := appws.NewCreateWorkspace(ws, members)
	w, err := wsCreate.Execute(context.Background(), "owner-1", "Private")
	require.NoError(t, err)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/workspaces/"+w.ID+"/members", nil)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestInviteMember(t *testing.T) {
	srv, _ := buildWSAPI(t, "owner-1")
	defer srv.Close()

	// create workspace
	body, _ := json.Marshal(map[string]string{"name": "Corp"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	wsID := created["id"].(string)

	// invite
	invBody, _ := json.Marshal(map[string]string{"email": "bob@example.com", "role": domainws.RoleMember})
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces/"+wsID+"/invites", bytes.NewReader(invBody))
	req2.Header.Set("Authorization", "Bearer t")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()
	require.Equal(t, http.StatusAccepted, resp2.StatusCode)
}

func TestInviteMemberInvalidRole(t *testing.T) {
	srv, _ := buildWSAPI(t, "owner-1")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"name": "Corp"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	wsID := created["id"].(string)

	invBody, _ := json.Marshal(map[string]string{"email": "x@x.com", "role": "superadmin"})
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces/"+wsID+"/invites", bytes.NewReader(invBody))
	req2.Header.Set("Authorization", "Bearer t")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp2.StatusCode)
}

func TestCreateWorkspaceEmptyName(t *testing.T) {
	srv, _ := buildWSAPI(t, "user-1")
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"name": "   "})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAcceptInviteBadToken(t *testing.T) {
	srv, _ := buildWSAPI(t, "user-1")
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/invites/bad-token/accept", nil)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// helper: create a workspace via the API and return its ID.
func createWorkspaceViaAPI(t *testing.T, srvURL, token, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name})
	req, _ := http.NewRequest(http.MethodPost, srvURL+"/workspaces", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	return created["id"].(string)
}

func TestCreateAndListOrgUnitsEndpoint(t *testing.T) {
	srv, _ := buildWSAPI(t, "owner-1")
	defer srv.Close()
	wsID := createWorkspaceViaAPI(t, srv.URL, "t", "Org Corp")

	// POST /workspaces/{id}/org-units → 201
	body, _ := json.Marshal(map[string]string{"name": "Sales"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces/"+wsID+"/org-units", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NotEmpty(t, created["id"])
	require.Equal(t, "Sales", created["name"])

	// GET /workspaces/{id}/org-units → 200 with 1 item
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/workspaces/"+wsID+"/org-units", nil)
	req2.Header.Set("Authorization", "Bearer t")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	var list []map[string]any
	json.NewDecoder(resp2.Body).Decode(&list)
	resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	require.Len(t, list, 1)
	require.Equal(t, "Sales", list[0]["name"])
}

func TestCreateOrgUnitForbiddenForNonManager(t *testing.T) {
	// Build server as owner-1, then manually add member-2 as a plain member, then
	// create a new server running as member-2 using the same shared fakes.
	members := newFakeMembersHTTP()
	ws := newFakeWSHTTP(members)
	invites := newFakeInvitesHTTP()
	mailer := &fakeWSMailer{}
	units := &fakeOrgUnitsHTTP{}
	positions := &fakePositionsHTTP{}

	// Seed the workspace as owner-1 directly (bypassing HTTP so we can share fakes).
	w, err := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Corp")
	require.NoError(t, err)

	// Add member-2 as a plain member.
	_ = appws.NewListMembers(members) // just to import; member is added directly
	require.NoError(t, members.Create(context.Background(), &domainws.Member{
		ID: "m2", WorkspaceID: w.ID, UserID: "member-2",
		Role: domainws.RoleMember, Status: domainws.StatusActive,
	}))

	// Build a server whose Hydra returns "member-2".
	hydra := &fakeHydra{introspectActive: true, introspectSubject: "member-2"}
	products2 := &fakeProductsHTTP{}
	wp2 := newFakeWorkspaceProductsHTTP()
	h := apphttp.NewWorkspaceHandlers(
		appws.NewCreateWorkspace(ws, members),
		appws.NewListMyWorkspaces(ws),
		appws.NewListMembers(members),
		appws.NewInviteMember(ws, members, invites, mailer, "https://acc.example"),
		appws.NewAcceptInvite(invites, members),
		appws.NewCreateOrgUnit(members, units),
		appws.NewListOrgUnits(members, units),
		appws.NewCreatePosition(members, positions),
		appws.NewListPositions(members, positions),
		appws.NewAssignMember(members, units, positions),
		appws.NewListProducts(products2),
		appws.NewEnableProduct(members, products2, wp2),
		appws.NewDisableProduct(members, wp2),
		appws.NewListEnabledProducts(members, wp2),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) { pr.Use(apphttp.RequireAuth(hydra)); h.Register(pr) })
	srv := httptest.NewServer(r)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"name": "Finance"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces/"+w.ID+"/org-units", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestAssignMemberEndpoint(t *testing.T) {
	srv, _ := buildWSAPI(t, "owner-1")
	defer srv.Close()
	wsID := createWorkspaceViaAPI(t, srv.URL, "t", "Assign Corp")

	// Create an org unit and a position.
	ouBody, _ := json.Marshal(map[string]string{"name": "Engineering"})
	ouReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces/"+wsID+"/org-units", bytes.NewReader(ouBody))
	ouReq.Header.Set("Authorization", "Bearer t")
	ouResp, err := http.DefaultClient.Do(ouReq)
	require.NoError(t, err)
	var ou map[string]any
	json.NewDecoder(ouResp.Body).Decode(&ou)
	ouResp.Body.Close()
	require.Equal(t, http.StatusCreated, ouResp.StatusCode)
	ouID := ou["id"].(string)

	posBody, _ := json.Marshal(map[string]string{"title": "Engineer"})
	posReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces/"+wsID+"/positions", bytes.NewReader(posBody))
	posReq.Header.Set("Authorization", "Bearer t")
	posResp, err := http.DefaultClient.Do(posReq)
	require.NoError(t, err)
	var pos map[string]any
	json.NewDecoder(posResp.Body).Decode(&pos)
	posResp.Body.Close()
	require.Equal(t, http.StatusCreated, posResp.StatusCode)
	posID := pos["id"].(string)

	// PUT /workspaces/{id}/members/{userId}/assignment → 204
	assignBody, _ := json.Marshal(map[string]string{"org_unit_id": ouID, "position_id": posID})
	assignReq, _ := http.NewRequest(http.MethodPut, srv.URL+"/workspaces/"+wsID+"/members/owner-1/assignment", bytes.NewReader(assignBody))
	assignReq.Header.Set("Authorization", "Bearer t")
	assignResp, err := http.DefaultClient.Do(assignReq)
	require.NoError(t, err)
	assignResp.Body.Close()
	require.Equal(t, http.StatusNoContent, assignResp.StatusCode)
}

func TestListProductsEndpoint(t *testing.T) {
	srv, _ := buildWSAPI(t, "user-1")
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/products", nil)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	var list []map[string]any
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, list, 2)
}

func TestEnableProductEndpoint(t *testing.T) {
	srv, _ := buildWSAPI(t, "owner-1")
	defer srv.Close()
	wsID := createWorkspaceViaAPI(t, srv.URL, "t", "Prod Corp")

	// POST /workspaces/{id}/products → 201
	body, _ := json.Marshal(map[string]string{"product_key": "papyrus"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces/"+wsID+"/products", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// GET /workspaces/{id}/products → 200 with 1 item
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/workspaces/"+wsID+"/products", nil)
	req2.Header.Set("Authorization", "Bearer t")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	var list []map[string]any
	json.NewDecoder(resp2.Body).Decode(&list)
	resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	require.Len(t, list, 1)
	require.Equal(t, "papyrus", list[0]["Key"])
}

func TestEnableProductForbiddenForNonManager(t *testing.T) {
	members := newFakeMembersHTTP()
	ws := newFakeWSHTTP(members)
	invites := newFakeInvitesHTTP()
	mailer := &fakeWSMailer{}
	units := &fakeOrgUnitsHTTP{}
	positions := &fakePositionsHTTP{}
	products := &fakeProductsHTTP{list: []*domainws.Product{{Key: "papyrus", Name: "Papyrus"}}}
	wp := newFakeWorkspaceProductsHTTP()

	w, err := appws.NewCreateWorkspace(ws, members).Execute(context.Background(), "owner-1", "Corp")
	require.NoError(t, err)
	require.NoError(t, members.Create(context.Background(), &domainws.Member{
		ID: "m2", WorkspaceID: w.ID, UserID: "member-2",
		Role: domainws.RoleMember, Status: domainws.StatusActive,
	}))

	hydra := &fakeHydra{introspectActive: true, introspectSubject: "member-2"}
	h := apphttp.NewWorkspaceHandlers(
		appws.NewCreateWorkspace(ws, members),
		appws.NewListMyWorkspaces(ws),
		appws.NewListMembers(members),
		appws.NewInviteMember(ws, members, invites, mailer, "https://acc.example"),
		appws.NewAcceptInvite(invites, members),
		appws.NewCreateOrgUnit(members, units),
		appws.NewListOrgUnits(members, units),
		appws.NewCreatePosition(members, positions),
		appws.NewListPositions(members, positions),
		appws.NewAssignMember(members, units, positions),
		appws.NewListProducts(products),
		appws.NewEnableProduct(members, products, wp),
		appws.NewDisableProduct(members, wp),
		appws.NewListEnabledProducts(members, wp),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) { pr.Use(apphttp.RequireAuth(hydra)); h.Register(pr) })
	srv := httptest.NewServer(r)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"product_key": "papyrus"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces/"+w.ID+"/products", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestEnableUnknownProductReturns404(t *testing.T) {
	srv, _ := buildWSAPI(t, "owner-1")
	defer srv.Close()
	wsID := createWorkspaceViaAPI(t, srv.URL, "t", "Unknown Prod Corp")

	body, _ := json.Marshal(map[string]string{"product_key": "ghost"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces/"+wsID+"/products", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDisableProductEndpoint(t *testing.T) {
	srv, _ := buildWSAPI(t, "owner-1")
	defer srv.Close()
	wsID := createWorkspaceViaAPI(t, srv.URL, "t", "Disable Corp")

	// Enable first
	body, _ := json.Marshal(map[string]string{"product_key": "papyrus"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/workspaces/"+wsID+"/products", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// DELETE /workspaces/{id}/products/{key} → 204
	req2, _ := http.NewRequest(http.MethodDelete, srv.URL+"/workspaces/"+wsID+"/products/papyrus", nil)
	req2.Header.Set("Authorization", "Bearer t")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()
	require.Equal(t, http.StatusNoContent, resp2.StatusCode)

	// GET should now be empty
	req3, _ := http.NewRequest(http.MethodGet, srv.URL+"/workspaces/"+wsID+"/products", nil)
	req3.Header.Set("Authorization", "Bearer t")
	resp3, err := http.DefaultClient.Do(req3)
	require.NoError(t, err)
	var list []map[string]any
	json.NewDecoder(resp3.Body).Decode(&list)
	resp3.Body.Close()
	require.Equal(t, http.StatusOK, resp3.StatusCode)
	require.Len(t, list, 0)
}
