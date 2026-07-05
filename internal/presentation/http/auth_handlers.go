package http

import (
	"errors"
	"html/template"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	appidentity "github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
)

// AuthHandlers implements the Hydra login/consent provider endpoints.
type AuthHandlers struct {
	authenticate *appidentity.Authenticate
	hydra        domain.HydraClient
	sessions     domain.SessionRepository
	tpl          *template.Template
}

func NewAuthHandlers(authenticate *appidentity.Authenticate, hydra domain.HydraClient,
	sessions domain.SessionRepository, tpl *template.Template) *AuthHandlers {
	return &AuthHandlers{authenticate: authenticate, hydra: hydra, sessions: sessions, tpl: tpl}
}

func (h *AuthHandlers) Register(r chi.Router) {
	r.Get("/login", h.getLogin)
	r.Post("/login", h.postLogin)
	r.Get("/consent", h.getConsent)
}

func (h *AuthHandlers) renderLogin(w http.ResponseWriter, challenge, errMsg string, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = h.tpl.ExecuteTemplate(w, "login.html", map[string]any{"Challenge": challenge, "Error": errMsg})
}

func (h *AuthHandlers) getLogin(w http.ResponseWriter, r *http.Request) {
	challenge := r.URL.Query().Get("login_challenge")
	req, err := h.hydra.GetLoginRequest(r.Context(), challenge)
	if err != nil {
		http.Error(w, "login flow error", http.StatusBadGateway)
		return
	}
	if req.Skip {
		redirect, err := h.hydra.AcceptLoginRequest(r.Context(), challenge, req.Subject, true)
		if err != nil {
			http.Error(w, "login flow error", http.StatusBadGateway)
			return
		}
		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}
	h.renderLogin(w, challenge, "", http.StatusOK)
}

func (h *AuthHandlers) postLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	challenge := r.PostForm.Get("login_challenge")
	email := r.PostForm.Get("email")
	password := r.PostForm.Get("password")

	u, err := h.authenticate.Execute(r.Context(), email, password)
	if err != nil {
		msg := "Invalid email or password"
		if errors.Is(err, domain.ErrEmailNotVerified) {
			msg = "Please verify your email first"
		}
		h.renderLogin(w, challenge, msg, http.StatusOK)
		return
	}

	redirect, err := h.hydra.AcceptLoginRequest(r.Context(), challenge, u.ID, true)
	if err != nil {
		http.Error(w, "login flow error", http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (h *AuthHandlers) getConsent(w http.ResponseWriter, r *http.Request) {
	challenge := r.URL.Query().Get("consent_challenge")
	req, err := h.hydra.GetConsentRequest(r.Context(), challenge)
	if err != nil {
		http.Error(w, "consent flow error", http.StatusBadGateway)
		return
	}

	// Guard: Subject must be set — if empty the consent flow is broken.
	if req.Subject == "" {
		http.Error(w, "consent flow error", http.StatusBadGateway)
		return
	}

	if !req.Client.Trusted {
		// Non-trusted client: reject via Hydra — no session created.
		redirect, err := h.hydra.RejectConsentRequest(r.Context(), challenge, "consent_required")
		if err != nil {
			http.Error(w, "consent flow error", http.StatusBadGateway)
			return
		}
		http.Redirect(w, r, redirect, http.StatusFound)
		return
	}

	// Trusted (first-party) client: auto-accept all requested scopes.
	redirect, err := h.hydra.AcceptConsentRequest(r.Context(), challenge, req.RequestedScopes)
	if err != nil {
		http.Error(w, "consent flow error", http.StatusBadGateway)
		return
	}

	// Record the session (sid available here as LoginSessionID).
	if err := h.sessions.Create(r.Context(), &domain.Session{
		ID:             uuid.NewString(),
		UserID:         req.Subject,
		HydraSessionID: req.LoginSessionID,
		DeviceName:     deviceFromUA(r.UserAgent()),
		UserAgent:      r.UserAgent(),
		IP:             clientIP(r),
	}); err != nil {
		log.Printf("consent: failed to create session for subject %q: %v", req.Subject, err)
	}

	http.Redirect(w, r, redirect, http.StatusFound)
}

func deviceFromUA(ua string) string {
	if ua == "" {
		return "Unknown device"
	}
	return ua // MVP: store raw UA; pretty parsing deferred
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}
