package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

func TestRouterServesHealthz(t *testing.T) {
	h := apphttp.NewIdentityHandlers(nil, nil, nil, nil)
	srv := httptest.NewServer(NewRouter(h))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}
