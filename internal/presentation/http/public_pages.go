package http

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// PublicPageHandlers render the public (unauthenticated) auth pages.
type PublicPageHandlers struct {
	register *appidentity.RegisterUser
	verify   *appidentity.VerifyEmail
	reqReset *appidentity.RequestPasswordReset
	reset    *appidentity.ResetPassword
	tpl      *template.Template
}

func NewPublicPageHandlers(register *appidentity.RegisterUser, verify *appidentity.VerifyEmail,
	reqReset *appidentity.RequestPasswordReset, reset *appidentity.ResetPassword,
	tpl *template.Template) *PublicPageHandlers {
	return &PublicPageHandlers{register: register, verify: verify, reqReset: reqReset, reset: reset, tpl: tpl}
}

func (h *PublicPageHandlers) Register(r chi.Router) {
	r.Get("/signup", h.signupForm)
	r.Post("/signup", h.signupSubmit)
	r.Get("/forgot-password", h.forgotForm)
	r.Post("/forgot-password", h.forgotSubmit)
	r.Get("/reset-password", h.resetForm)
	r.Post("/reset-password", h.resetSubmit)
	r.Get("/verify-email", h.verifyEmail)
}

func (h *PublicPageHandlers) render(w http.ResponseWriter, name string, data any, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = h.tpl.ExecuteTemplate(w, name, data)
}

func (h *PublicPageHandlers) msg(w http.ResponseWriter, title, message string) {
	h.render(w, "message.html", map[string]any{"Title": title, "Message": message}, http.StatusOK)
}

func (h *PublicPageHandlers) signupForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "signup.html", map[string]any{"Error": ""}, http.StatusOK)
}

func (h *PublicPageHandlers) signupSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	_, err := h.register.Execute(r.Context(), appidentity.RegisterInput{
		Email: r.PostForm.Get("email"), Password: r.PostForm.Get("password"), Name: r.PostForm.Get("name"),
	})
	if err != nil {
		h.render(w, "signup.html", map[string]any{"Error": humanizeAuthError(err)}, http.StatusOK)
		return
	}
	h.msg(w, "Check your email", "We sent a verification link to your email address.")
}

func (h *PublicPageHandlers) forgotForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "forgot_password.html", nil, http.StatusOK)
}

func (h *PublicPageHandlers) forgotSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	_ = h.reqReset.Execute(r.Context(), r.PostForm.Get("email")) // silent regardless
	h.msg(w, "Check your email", "If an account exists for that email, a reset link has been sent.")
}

func (h *PublicPageHandlers) resetForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "reset_password.html", map[string]any{"Token": r.URL.Query().Get("token"), "Error": ""}, http.StatusOK)
}

func (h *PublicPageHandlers) resetSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	token := r.PostForm.Get("token")
	if err := h.reset.Execute(r.Context(), token, r.PostForm.Get("password")); err != nil {
		h.render(w, "reset_password.html", map[string]any{"Token": token, "Error": humanizeAuthError(err)}, http.StatusOK)
		return
	}
	h.msg(w, "Password changed", "Your password has been updated. You can now log in.")
}

func (h *PublicPageHandlers) verifyEmail(w http.ResponseWriter, r *http.Request) {
	if err := h.verify.Execute(r.Context(), r.URL.Query().Get("token")); err != nil {
		h.msg(w, "Verification failed", "This link is invalid or has expired.")
		return
	}
	h.msg(w, "Email verified", "Your email has been verified. You can now log in.")
}

func humanizeAuthError(err error) string {
	switch {
	case errors.Is(err, domain.ErrUserExists):
		return "An account with this email already exists."
	case errors.Is(err, domain.ErrWeakPassword):
		return "Password is too weak (min 8 characters)."
	case errors.Is(err, domain.ErrInvalidEmail):
		return "Please enter a valid email."
	case errors.Is(err, domain.ErrTokenInvalid):
		return "This link is invalid or has expired."
	default:
		return "Something went wrong. Please try again."
	}
}
