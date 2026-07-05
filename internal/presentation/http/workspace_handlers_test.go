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
	h := apphttp.NewWorkspaceHandlers(
		appws.NewCreateWorkspace(ws, members),
		appws.NewListMyWorkspaces(ws),
		appws.NewListMembers(members),
		appws.NewInviteMember(ws, members, invites, mailer, "https://acc.example"),
		appws.NewAcceptInvite(invites, members),
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
