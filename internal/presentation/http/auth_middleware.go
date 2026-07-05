package http

import (
	"context"
	"net/http"
	"strings"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

type ctxKey int

const userIDKey ctxKey = 0

// RequireAuth validates a Bearer access token via Hydra introspection and puts
// the subject (user id) into the request context.
func RequireAuth(hydra domain.HydraClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(authz, prefix) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			token := strings.TrimSpace(authz[len(prefix):])
			active, subject, err := hydra.IntrospectToken(r.Context(), token)
			if err != nil {
				http.Error(w, "auth error", http.StatusBadGateway)
				return
			}
			if !active || subject == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userIDKey, subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext returns the authenticated user id, or "" if none.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey).(string); ok {
		return v
	}
	return ""
}
