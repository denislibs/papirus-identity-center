package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

// fakes for hub auth
type fakeHubOAuth struct {
	state       string
	exchangeSub string
	exchangeErr error
}

func (f *fakeHubOAuth) AuthCodeURL(state string) string { f.state = state; return "https://hydra/auth?state=" + state }
func (f *fakeHubOAuth) ExchangeForSubject(_ context.Context, _ string) (string, error) {
	return f.exchangeSub, f.exchangeErr
}

type fakeHubStore struct {
	created string
	deleted string
	id      string
}

func (f *fakeHubStore) Create(_ context.Context, subject string) (string, error) {
	f.created = subject
	if f.id == "" {
		f.id = "hubid-1"
	}
	return f.id, nil
}
func (f *fakeHubStore) Subject(_ context.Context, id string) (string, error) { return f.created, nil }
func (f *fakeHubStore) Delete(_ context.Context, id string) error            { f.deleted = id; return nil }

func newHubAuthServer(t *testing.T) (*httptest.Server, *fakeHubOAuth, *fakeHubStore) {
	t.Helper()
	oauth := &fakeHubOAuth{exchangeSub: "user-1"}
	store := &fakeHubStore{}
	h := apphttp.NewHubAuthHandlers(oauth, store)
	r := chi.NewRouter()
	h.Register(r)
	return httptest.NewServer(r), oauth, store
}

func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func TestHubLoginRedirectsToHydraWithState(t *testing.T) {
	srv, _, _ := newHubAuthServer(t)
	defer srv.Close()
	resp, err := noRedirect().Get(srv.URL + "/auth/login")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.True(t, strings.HasPrefix(resp.Header.Get("Location"), "https://hydra/auth?state="))
	// state cookie set
	require.NotEmpty(t, resp.Cookies())
}

func TestHubCallbackCreatesSessionAndCookie(t *testing.T) {
	srv, oauth, store := newHubAuthServer(t)
	defer srv.Close()

	// first hit /auth/login to obtain a state + cookie
	loginResp, err := noRedirect().Get(srv.URL + "/auth/login")
	require.NoError(t, err)
	loginResp.Body.Close()
	stateCookie := loginResp.Cookies()[0]
	state := oauth.state

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/callback?code=abc&state="+state, nil)
	req.AddCookie(stateCookie)
	resp, err := noRedirect().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "/account", resp.Header.Get("Location"))
	require.Equal(t, "user-1", store.created)
	// hub_session cookie set
	var hubCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "hub_session" {
			hubCookie = c
		}
	}
	require.NotNil(t, hubCookie)
	require.Equal(t, "hubid-1", hubCookie.Value)
	require.True(t, hubCookie.HttpOnly)
}

func TestHubCallbackRejectsStateMismatch(t *testing.T) {
	srv, _, store := newHubAuthServer(t)
	defer srv.Close()
	loginResp, _ := noRedirect().Get(srv.URL + "/auth/login")
	loginResp.Body.Close()
	stateCookie := loginResp.Cookies()[0]

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/callback?code=abc&state=WRONG", nil)
	req.AddCookie(stateCookie)
	resp, err := noRedirect().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Empty(t, store.created) // no session created
}
