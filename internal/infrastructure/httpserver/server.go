package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	domainidentity "github.com/denislibs/papirus-identity-center/internal/domain/identity"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
)

// NewRouter wires HTTP routes for the platform.
func NewRouter(identity *apphttp.IdentityHandlers, auth *apphttp.AuthHandlers,
	sessions *apphttp.SessionHandlers, hydra domainidentity.HydraClient,
	hubAuth *apphttp.HubAuthHandlers, hub *apphttp.HubHandlers, hubStore domainidentity.HubSessionStore,
	public *apphttp.PublicPageHandlers) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", apphttp.Healthz())
	public.Register(r) // public auth HTML pages (signup, forgot, reset, verify)
	identity.Register(r)
	auth.Register(r)
	hubAuth.Register(r) // /auth/login, /auth/callback, /auth/logout (public)

	// Authenticated platform API.
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireAuth(hydra))
		sessions.Register(pr)
	})

	// Hub pages (cookie session).
	r.Group(func(pr chi.Router) {
		pr.Use(apphttp.RequireHubSession(hubStore))
		hub.Register(pr)
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
