package http_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

func buildAuthServer(t *testing.T) (*httptest.Server, *fakeHydra, *fakeSessions, *fakeUsers) {
	t.Helper()
	users := newFakeUsers()
	sessions := &fakeSessions{}
	hydra := &fakeHydra{redirect: "https://hydra/redirect"}
	h := apphttp.NewAuthHandlers(
		appidentity.NewAuthenticate(users, fakeHasher{}),
		hydra, sessions, apphttp.MustLoadTemplates(),
	)
	r := chi.NewRouter()
	h.Register(r)
	// client that does NOT auto-follow redirects
	srv := httptest.NewServer(r)
	return srv, hydra, sessions, users
}

func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func TestLoginGETRendersForm(t *testing.T) {
	srv, _, _, _ := buildAuthServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/login?login_challenge=chal-1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	require.Contains(t, string(body[:n]), `value="chal-1"`)
}

func TestLoginPOSTSuccessAcceptsAndRedirects(t *testing.T) {
	srv, hydra, _, users := buildAuthServer(t)
	defer srv.Close()
	_ = users.Create(nil, &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "hashed:pw", EmailVerified: true})

	form := url.Values{"login_challenge": {"chal-1"}, "email": {"a@x.com"}, "password": {"pw"}}
	resp, err := noRedirectClient().PostForm(srv.URL+"/login", form)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode) // 302
	require.Equal(t, "https://hydra/redirect", resp.Header.Get("Location"))
	require.Equal(t, "u1", hydra.acceptedSub)
}

func TestLoginPOSTBadPasswordRerendersForm(t *testing.T) {
	srv, _, _, users := buildAuthServer(t)
	defer srv.Close()
	_ = users.Create(nil, &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "hashed:pw", EmailVerified: true})

	form := url.Values{"login_challenge": {"chal-1"}, "email": {"a@x.com"}, "password": {"WRONG"}}
	resp, err := noRedirectClient().PostForm(srv.URL+"/login", form)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode) // re-render, not redirect
	body := make([]byte, 8192)
	n, _ := resp.Body.Read(body)
	require.True(t, strings.Contains(string(body[:n]), "form")) // form shown again
}

func TestConsentAutoAcceptsTrustedAndCreatesSession(t *testing.T) {
	srv, hydra, sessions, _ := buildAuthServer(t)
	defer srv.Close()
	hydra.consent = &domain.HydraConsentRequest{
		Subject: "u1", LoginSessionID: "sid-xyz",
		RequestedScopes: []string{"openid", "profile"},
		Client:          domain.OAuthClientInfo{ID: "papyrus", Trusted: true},
	}

	resp, err := noRedirectClient().Get(srv.URL + "/consent?consent_challenge=cc-1")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, []string{"openid", "profile"}, hydra.grantedScopes)
	require.False(t, hydra.consentRejected)
	require.Len(t, sessions.created, 1)
	require.Equal(t, "u1", sessions.created[0].UserID)
	require.Equal(t, "sid-xyz", sessions.created[0].HydraSessionID)
}

func TestConsentNonTrustedClientRejectedNoSession(t *testing.T) {
	srv, hydra, sessions, _ := buildAuthServer(t)
	defer srv.Close()
	hydra.consent = &domain.HydraConsentRequest{
		Subject:         "u1",
		LoginSessionID:  "sid-abc",
		RequestedScopes: []string{"openid"},
		Client:          domain.OAuthClientInfo{ID: "third-party", Trusted: false},
	}

	resp, err := noRedirectClient().Get(srv.URL + "/consent?consent_challenge=cc-2")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "https://hydra/redirect", resp.Header.Get("Location"))
	require.True(t, hydra.consentRejected, "expected consent to be rejected for non-trusted client")
	require.Len(t, sessions.created, 0, "no session should be created for non-trusted client")
}

func TestConsentEmptySubjectReturns502(t *testing.T) {
	srv, hydra, sessions, _ := buildAuthServer(t)
	defer srv.Close()
	hydra.consent = &domain.HydraConsentRequest{
		Subject:         "", // empty — consent flow is broken
		LoginSessionID:  "sid-bad",
		RequestedScopes: []string{"openid"},
		Client:          domain.OAuthClientInfo{ID: "papyrus", Trusted: true},
	}

	resp, err := noRedirectClient().Get(srv.URL + "/consent?consent_challenge=cc-3")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	require.Len(t, sessions.created, 0, "no session should be created when Subject is empty")
}
