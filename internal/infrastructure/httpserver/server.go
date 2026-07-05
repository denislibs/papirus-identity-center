package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

// NewRouter wires HTTP routes for the platform.
func NewRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", apphttp.Healthz())

	return r
}

// NewServer builds an *http.Server listening on the given address.
func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: handler,
	}
}
