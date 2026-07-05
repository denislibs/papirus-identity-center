package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	domainidentity "github.com/papyrus/platform/internal/domain/identity"
	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

// NewRouter wires HTTP routes for the platform.
func NewRouter(identity *apphttp.IdentityHandlers, auth *apphttp.AuthHandlers,
	sessions *apphttp.SessionHandlers, hydra domainidentity.HydraClient) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", apphttp.Healthz())
	identity.Register(r)
	auth.Register(r)

	// Authenticated platform API.
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireAuth(hydra))
		sessions.Register(pr)
	})

	return r
}

// NewServer builds an *http.Server listening on the given address.
func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: handler,
	}
}
