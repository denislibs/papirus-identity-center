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
	store := &fakeHubStore{created: "user-9", id: "hubid-9"}

	h := apphttp.NewHubHandlers(appidentity.NewGetProfile(users), apphttp.MustLoadTemplates())
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
