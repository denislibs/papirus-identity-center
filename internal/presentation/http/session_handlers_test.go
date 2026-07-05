package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

func buildSessionAPI(t *testing.T, userID string) (*httptest.Server, *fakeSessions, *fakeHydra) {
	t.Helper()
	sessions := &fakeSessions{}
	hydra := &fakeHydra{introspectActive: true, introspectSubject: userID}
	h := apphttp.NewSessionHandlers(
		appidentity.NewListSessions(sessions),
		appidentity.NewTerminateSession(sessions, hydra),
		appidentity.NewTerminateAllSessions(sessions, hydra),
	)
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireAuth(hydra))
		h.Register(pr)
	})
	return httptest.NewServer(r), sessions, hydra
}

func TestListSessionsEndpoint(t *testing.T) {
	srv, sessions, _ := buildSessionAPI(t, "u1")
	defer srv.Close()
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "u1", HydraSessionID: "sid1", DeviceName: "Chrome"})

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var out []map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Len(t, out, 1)
	require.Equal(t, "s1", out[0]["id"])
}

func TestDeleteSessionEndpoint(t *testing.T) {
	srv, sessions, hydra := buildSessionAPI(t, "u1")
	defer srv.Close()
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "u1", HydraSessionID: "sid1"})

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/sessions/s1", nil)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, "sid1", hydra.revokedSID)
}

func TestLogoutAllEndpoint(t *testing.T) {
	srv, _, hydra := buildSessionAPI(t, "u1")
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/sessions/logout-all", nil)
	req.Header.Set("Authorization", "Bearer t")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, "u1", hydra.revokedSubject)
}

func TestSessionAPIRequiresAuth(t *testing.T) {
	srv, _, _ := buildSessionAPI(t, "u1")
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/sessions") // no token
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
