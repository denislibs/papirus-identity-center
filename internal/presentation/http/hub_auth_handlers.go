package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/go-chi/chi/v5"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// HubOAuth is the OAuth client behavior the hub needs.
type HubOAuth interface {
	AuthCodeURL(state string) string
	ExchangeForSubject(ctx context.Context, code string) (string, error)
}

const (
	stateCookie   = "hub_oauth_state"
	sessionCookie = "hub_session"
)

// HubAuthHandlers implement the hub's OIDC-client login/callback/logout.
type HubAuthHandlers struct {
	oauth HubOAuth
	store domain.HubSessionStore
}

func NewHubAuthHandlers(oauth HubOAuth, store domain.HubSessionStore) *HubAuthHandlers {
	return &HubAuthHandlers{oauth: oauth, store: store}
}

func (h *HubAuthHandlers) Register(r chi.Router) {
	r.Get("/auth/login", h.login)
	r.Get("/auth/callback", h.callback)
	r.Get("/auth/logout", h.logout)
}

func randToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (h *HubAuthHandlers) login(w http.ResponseWriter, r *http.Request) {
	state, err := randToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 300,
	})
	http.Redirect(w, r, h.oauth.AuthCodeURL(state), http.StatusFound)
}

func (h *HubAuthHandlers) callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(stateCookie)
	if err != nil || c.Value == "" || c.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	subject, err := h.oauth.ExchangeForSubject(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "login failed", http.StatusBadGateway)
		return
	}
	id, err := h.store.Create(r.Context(), subject)
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	// clear state cookie, set session cookie
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: id, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/account", http.StatusFound)
}

func (h *HubAuthHandlers) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		_ = h.store.Delete(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/account", http.StatusFound)
}
