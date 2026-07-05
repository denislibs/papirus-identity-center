package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

func TestRequireHubSessionRedirectsWhenNoCookie(t *testing.T) {
	store := &fakeHubStore{}
	mw := apphttp.RequireHubSession(store)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "/auth/login", rec.Header().Get("Location"))
}

func TestRequireHubSessionPassesAndSetsUser(t *testing.T) {
	store := &fakeHubStore{created: "user-9", id: "hubid-9"}
	mw := apphttp.RequireHubSession(store)
	var seen string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = apphttp.HubUserIDFromContext(r.Context())
		w.WriteHeader(200)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/account", nil)
	req.AddCookie(&http.Cookie{Name: "hub_session", Value: "hubid-9"})
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "user-9", seen)
}
