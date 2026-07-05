package http

import (
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
)

// HubHandlers render the authenticated account hub pages.
type HubHandlers struct {
	profile      *appidentity.GetProfile
	listSessions *appidentity.ListSessions
	terminate    *appidentity.TerminateSession
	terminateAll *appidentity.TerminateAllSessions
	tpl          *template.Template
}

func NewHubHandlers(profile *appidentity.GetProfile, list *appidentity.ListSessions,
	terminate *appidentity.TerminateSession, terminateAll *appidentity.TerminateAllSessions,
	tpl *template.Template) *HubHandlers {
	return &HubHandlers{profile: profile, listSessions: list, terminate: terminate, terminateAll: terminateAll, tpl: tpl}
}

func (h *HubHandlers) Register(r chi.Router) {
	r.Get("/account", h.account)
	r.Get("/account/sessions", h.sessions)
	r.Post("/account/sessions/{id}/terminate", h.terminateSession)
	r.Post("/account/sessions/logout-all", h.logoutAll)
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

func (h *HubHandlers) sessions(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	list, err := h.listSessions.Execute(r.Context(), userID)
	if err != nil {
		http.Error(w, "sessions error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tpl.ExecuteTemplate(w, "sessions.html", map[string]any{"Sessions": list})
}

func (h *HubHandlers) terminateSession(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")
	_ = h.terminate.Execute(r.Context(), userID, id) // ownership enforced in use-case; ignore not-found on redirect
	http.Redirect(w, r, "/account/sessions", http.StatusSeeOther)
}

func (h *HubHandlers) logoutAll(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	_ = h.terminateAll.Execute(r.Context(), userID)
	http.Redirect(w, r, "/account/sessions", http.StatusSeeOther)
}
