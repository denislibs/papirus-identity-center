package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

func TestRouterServesHealthz(t *testing.T) {
	identity := apphttp.NewIdentityHandlers(nil, nil, nil, nil)
	auth := apphttp.NewAuthHandlers(nil, nil, nil, apphttp.MustLoadTemplates())
	srv := httptest.NewServer(NewRouter(identity, auth))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}
