package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

func TestAccountPageRendersProfile(t *testing.T) {
	users := newFakeUsers()
	_ = users.Create(context.Background(), &domain.User{ID: "user-9", Email: "me@x.com", Name: "Me"})
	sessions := &fakeSessions{}
	hydra := &fakeHydra{}
	store := &fakeHubStore{created: "user-9", id: "hubid-9"}

	h := apphttp.NewHubHandlers(
		appidentity.NewGetProfile(users),
		appidentity.NewListSessions(sessions),
		appidentity.NewTerminateSession(sessions, hydra),
		appidentity.NewTerminateAllSessions(sessions, hydra),
		apphttp.MustLoadTemplates(),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireHubSession(store))
		h.Register(pr)
	})
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/account", nil)
	req.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-9"})
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	out := string(body[:n])
	require.True(t, strings.Contains(out, "me@x.com"))
	require.True(t, strings.Contains(out, "Me"))
}

func TestSessionsPageListsAndTerminates(t *testing.T) {
	users := newFakeUsers()
	_ = users.Create(context.Background(), &domain.User{ID: "user-9", Email: "me@x.com", Name: "Me"})
	sessions := &fakeSessions{}
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "user-9", HydraSessionID: "sid1", DeviceName: "Chrome", IP: "1.2.3.4"})
	hydra := &fakeHydra{}
	store := &fakeHubStore{created: "user-9", id: "hubid-9"}

	h := apphttp.NewHubHandlers(
		appidentity.NewGetProfile(users),
		appidentity.NewListSessions(sessions),
		appidentity.NewTerminateSession(sessions, hydra),
		appidentity.NewTerminateAllSessions(sessions, hydra),
		apphttp.MustLoadTemplates(),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireHubSession(store))
		h.Register(pr)
	})
	srv := httptest.NewServer(r)
	defer srv.Close()
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// list
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/account/sessions", nil)
	req.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-9"})
	resp, err := cl.Do(req)
	require.NoError(t, err)
	body := make([]byte, 8192)
	n, _ := resp.Body.Read(body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, string(body[:n]), "Chrome")

	// terminate one
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/account/sessions/s1/terminate", nil)
	req2.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-9"})
	resp2, err := cl.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp2.StatusCode)
	require.Equal(t, "sid1", hydra.revokedSID)
}

func TestSessionsLogoutAll(t *testing.T) {
	users := newFakeUsers()
	sessions := &fakeSessions{}
	hydra := &fakeHydra{}
	store := &fakeHubStore{created: "user-9", id: "hubid-9"}
	h := apphttp.NewHubHandlers(
		appidentity.NewGetProfile(users), appidentity.NewListSessions(sessions),
		appidentity.NewTerminateSession(sessions, hydra), appidentity.NewTerminateAllSessions(sessions, hydra),
		apphttp.MustLoadTemplates(),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) { pr.Use(apphttp.RequireHubSession(store)); h.Register(pr) })
	srv := httptest.NewServer(r)
	defer srv.Close()
	cl := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/account/sessions/logout-all", nil)
	req.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-9"})
	resp, err := cl.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusSeeOther, resp.StatusCode)
	require.Equal(t, "user-9", hydra.revokedSubject)
}
