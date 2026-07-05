package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/papyrus/platform/internal/infrastructure/hydra"
	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

func TestRouterServesHealthz(t *testing.T) {
	identity := apphttp.NewIdentityHandlers(nil, nil, nil, nil)
	auth := apphttp.NewAuthHandlers(nil, nil, nil, apphttp.MustLoadTemplates())
	sessions := apphttp.NewSessionHandlers(nil, nil, nil)
	hydraClient := hydra.New("http://localhost:0", nil)
	srv := httptest.NewServer(NewRouter(identity, auth, sessions, hydraClient))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}
