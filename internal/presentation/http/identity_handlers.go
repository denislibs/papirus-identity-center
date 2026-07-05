package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	appidentity "github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
)

// IdentityHandlers exposes JSON endpoints for registration and password reset.
type IdentityHandlers struct {
	register *appidentity.RegisterUser
	verify   *appidentity.VerifyEmail
	reqReset *appidentity.RequestPasswordReset
	reset    *appidentity.ResetPassword
}

func NewIdentityHandlers(register *appidentity.RegisterUser, verify *appidentity.VerifyEmail,
	reqReset *appidentity.RequestPasswordReset, reset *appidentity.ResetPassword) *IdentityHandlers {
	return &IdentityHandlers{register: register, verify: verify, reqReset: reqReset, reset: reset}
}

// Register mounts the identity routes on r.
func (h *IdentityHandlers) Register(r chi.Router) {
	r.Post("/register", h.handleRegister)
	r.Get("/verify-email", h.handleVerifyEmail)
	r.Post("/password-reset/request", h.handleRequestReset)
	r.Post("/password-reset/confirm", h.handleConfirmReset)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrUserExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "user already exists"})
	case errors.Is(err, domain.ErrWeakPassword):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password too weak"})
	case errors.Is(err, domain.ErrInvalidEmail):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email"})
	case errors.Is(err, domain.ErrTokenInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token invalid or expired"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (h *IdentityHandlers) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email, Password, Name, Locale, Timezone string
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	u, err := h.register.Execute(r.Context(), appidentity.RegisterInput{
		Email: body.Email, Password: body.Password, Name: body.Name,
		Locale: body.Locale, Timezone: body.Timezone,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": u.ID, "email": u.Email})
}

func (h *IdentityHandlers) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if err := h.verify.Execute(r.Context(), token); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

func (h *IdentityHandlers) handleRequestReset(w http.ResponseWriter, r *http.Request) {
	var body struct{ Email string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := h.reqReset.Execute(r.Context(), body.Email); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *IdentityHandlers) handleConfirmReset(w http.ResponseWriter, r *http.Request) {
	var body struct{ Token, Password string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if err := h.reset.Execute(r.Context(), body.Token, body.Password); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}
