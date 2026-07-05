package http

import (
	"context"
	"net/http"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

type hubCtxKey int

const hubUserKey hubCtxKey = 0

// RequireHubSession loads the hub session from the cookie; redirects to /auth/login
// if absent/invalid, else puts the subject (user id) into context.
func RequireHubSession(store domain.HubSessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(sessionCookie)
			if err != nil || c.Value == "" {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}
			subject, err := store.Subject(r.Context(), c.Value)
			if err != nil || subject == "" {
				http.Redirect(w, r, "/auth/login", http.StatusFound)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), hubUserKey, subject)))
		})
	}
}

// HubUserIDFromContext returns the hub-authenticated user id, or "".
func HubUserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(hubUserKey).(string); ok {
		return v
	}
	return ""
}
