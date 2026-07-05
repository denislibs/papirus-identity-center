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

	appidentity "github.com/papyrus/platform/internal/application/identity"
	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

// buildTestServer wires the identity use-cases with in-memory fakes into a chi router.
func buildTestServer(t *testing.T) (*httptest.Server, *fakeMailer) {
	t.Helper()
	users := newFakeUsers()
	tokens := newFakeTokens()
	mailer := newFakeMailer()
	hasher := fakeHasher{}
	base := "https://acc.example"

	h := apphttp.NewIdentityHandlers(
		appidentity.NewRegisterUser(users, hasher, tokens, mailer, base),
		appidentity.NewVerifyEmail(users, tokens),
		appidentity.NewRequestPasswordReset(users, tokens, mailer, base),
		appidentity.NewResetPassword(users, hasher, tokens),
	)
	r := chi.NewRouter()
	h.Register(r)
	return httptest.NewServer(r), mailer
}

func TestRegisterEndpoint(t *testing.T) {
	srv, mailer := buildTestServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"email": "a@x.com", "password": "long-enough-pw", "name": "Al"})
	resp, err := http.Post(srv.URL+"/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.NotEmpty(t, out["id"])
	require.Equal(t, "a@x.com", out["email"])
	require.Len(t, mailer.verifications, 1)
}

func TestRegisterEndpointRejectsWeakPassword(t *testing.T) {
	srv, _ := buildTestServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"email": "a@x.com", "password": "short"})
	resp, err := http.Post(srv.URL+"/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPasswordResetRequestAlwaysAccepted(t *testing.T) {
	srv, _ := buildTestServer(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"email": "ghost@x.com"})
	resp, err := http.Post(srv.URL+"/password-reset/request", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode) // 202 even for unknown email
}

// fakes reused via a local copy in this package's test files (see fakes_test.go in this package).
var _ = context.Background
