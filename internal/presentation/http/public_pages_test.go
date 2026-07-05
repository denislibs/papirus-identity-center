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

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

func buildPublicPages(t *testing.T) (*httptest.Server, *fakeUsers, *fakeTokens, *fakeMailer) {
	t.Helper()
	users := newFakeUsers()
	tokens := newFakeTokens()
	mailer := newFakeMailer()
	base := "https://acc.example"
	h := apphttp.NewPublicPageHandlers(
		appidentity.NewRegisterUser(users, fakeHasher{}, tokens, mailer, base),
		appidentity.NewVerifyEmail(users, tokens),
		appidentity.NewRequestPasswordReset(users, tokens, mailer, base),
		appidentity.NewResetPassword(users, fakeHasher{}, tokens),
		apphttp.MustLoadTemplates(),
	)
	r := chi.NewRouter()
	h.Register(r)
	return httptest.NewServer(r), users, tokens, mailer
}

func getBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b := make([]byte, 8192)
	n, _ := resp.Body.Read(b)
	resp.Body.Close()
	return string(b[:n])
}

func TestSignupGETShowsForm(t *testing.T) {
	srv, _, _, _ := buildPublicPages(t)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/signup")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, getBody(t, resp), `action="/signup"`)
}

func TestSignupPOSTRegistersAndSendsMail(t *testing.T) {
	srv, _, _, mailer := buildPublicPages(t)
	defer srv.Close()
	form := url.Values{"email": {"new@x.com"}, "password": {"long-enough-pw"}, "name": {"New"}}
	resp, err := http.PostForm(srv.URL+"/signup", form)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, mailer.verifications, 1)
	require.Contains(t, getBody(t, resp), "email") // "check your email" message
}

func TestSignupPOSTWeakPasswordShowsError(t *testing.T) {
	srv, _, _, _ := buildPublicPages(t)
	defer srv.Close()
	form := url.Values{"email": {"x@x.com"}, "password": {"short"}}
	resp, err := http.PostForm(srv.URL+"/signup", form)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode) // re-render form with error
	require.Contains(t, strings.ToLower(getBody(t, resp)), "password")
}

func TestVerifyEmailPageMarksVerified(t *testing.T) {
	srv, users, tokens, _ := buildPublicPages(t)
	defer srv.Close()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com"})
	tok, _ := tokens.Issue(context.Background(), domain.PurposeVerifyEmail, "u1", 0)
	resp, err := http.Get(srv.URL + "/verify-email?token=" + tok)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got, _ := users.FindByID(context.Background(), "u1")
	require.True(t, got.EmailVerified)
}

func TestResetPasswordFlow(t *testing.T) {
	srv, users, tokens, _ := buildPublicPages(t)
	defer srv.Close()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "old"})
	tok, _ := tokens.Issue(context.Background(), domain.PurposePasswordReset, "u1", 0)

	// GET form shows token
	resp, err := http.Get(srv.URL + "/reset-password?token=" + tok)
	require.NoError(t, err)
	require.Contains(t, getBody(t, resp), tok)

	// POST new password
	form := url.Values{"token": {tok}, "password": {"brand-new-pw"}}
	resp2, err := http.PostForm(srv.URL+"/reset-password", form)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	got, _ := users.FindByID(context.Background(), "u1")
	require.Equal(t, "hashed:brand-new-pw", got.PasswordHash)
}
