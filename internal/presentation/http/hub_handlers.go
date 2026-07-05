package http

import (
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
)

// HubHandlers render the authenticated account hub pages.
type HubHandlers struct {
	profile *appidentity.GetProfile
	tpl     *template.Template
}

func NewHubHandlers(profile *appidentity.GetProfile, tpl *template.Template) *HubHandlers {
	return &HubHandlers{profile: profile, tpl: tpl}
}

// Register mounts hub pages (expects RequireHubSession applied by caller).
func (h *HubHandlers) Register(r chi.Router) {
	r.Get("/account", h.account)
}

func (h *HubHandlers) account(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	u, err := h.profile.Execute(r.Context(), userID)
	if err != nil {
		http.Error(w, "profile error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tpl.ExecuteTemplate(w, "account.html", map[string]any{"Email": u.Email, "Name": u.Name})
}
