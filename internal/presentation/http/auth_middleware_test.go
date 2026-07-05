package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	mw := apphttp.RequireAuth(&fakeHydra{})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireAuthRejectsInactiveToken(t *testing.T) {
	hydra := &fakeHydra{introspectActive: false}
	mw := apphttp.RequireAuth(hydra)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireAuthPassesAndSetsUser(t *testing.T) {
	hydra := &fakeHydra{introspectActive: true, introspectSubject: "user-42"}
	mw := apphttp.RequireAuth(hydra)

	var seen string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = apphttp.UserIDFromContext(r.Context())
		w.WriteHeader(200)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer good")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "user-42", seen)
}
