package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/denislibs/papirus-identity-center/internal/infrastructure/hydra"
	rdc "github.com/denislibs/papirus-identity-center/internal/infrastructure/redis"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

func TestRouterServesHealthz(t *testing.T) {
	identity := apphttp.NewIdentityHandlers(nil, nil, nil, nil)
	auth := apphttp.NewAuthHandlers(nil, nil, nil, apphttp.MustLoadTemplates())
	sessions := apphttp.NewSessionHandlers(nil, nil, nil)
	hydraClient := hydra.New("http://localhost:0", nil)
	hubAuth := apphttp.NewHubAuthHandlers(nil, nil)
	hub := apphttp.NewHubHandlers(nil, nil, nil, nil, apphttp.MustLoadTemplates())
	hubStore := rdc.NewHubSessionStore(nil, time.Hour)
	public := apphttp.NewPublicPageHandlers(nil, nil, nil, nil, apphttp.MustLoadTemplates())
	ws := apphttp.NewWorkspaceHandlers(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	srv := httptest.NewServer(NewRouter(identity, auth, sessions, hydraClient, hubAuth, hub, hubStore, public, ws))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}
