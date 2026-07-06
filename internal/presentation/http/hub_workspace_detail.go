package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *HubWorkspaceHandlers) registerDetail(r chi.Router) {
	r.Get("/account/workspaces/{id}", h.detail)
	r.Post("/account/workspaces/{id}/invites", h.inviteMember)
	r.Post("/account/workspaces/{id}/org-units", h.addUnit)
	r.Post("/account/workspaces/{id}/positions", h.addPosition)
	r.Post("/account/workspaces/{id}/products", h.enableProd)
}

func (h *HubWorkspaceHandlers) detail(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	wsID := chi.URLParam(r, "id")

	members, err := h.listMembers.Execute(r.Context(), wsID, userID)
	if err != nil {
		// not a member (or gone) → back to list
		http.Redirect(w, r, "/account/workspaces", http.StatusSeeOther)
		return
	}
	units, _ := h.listUnits.Execute(r.Context(), wsID, userID)
	positions, _ := h.listPos.Execute(r.Context(), wsID, userID)
	enabled, _ := h.listEnabled.Execute(r.Context(), wsID, userID)
	registry, _ := h.listProducts.Execute(r.Context())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tpl.ExecuteTemplate(w, "workspace_detail.html", map[string]any{
		"WorkspaceID": wsID, "Members": members, "Units": units,
		"Positions": positions, "Enabled": enabled, "Registry": registry,
	})
}

func (h *HubWorkspaceHandlers) inviteMember(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	wsID := chi.URLParam(r, "id")
	_ = h.invite.Execute(r.Context(), wsID, HubUserIDFromContext(r.Context()), r.PostForm.Get("email"), r.PostForm.Get("role"))
	http.Redirect(w, r, "/account/workspaces/"+wsID, http.StatusSeeOther)
}

func (h *HubWorkspaceHandlers) addUnit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	wsID := chi.URLParam(r, "id")
	_, _ = h.createUnit.Execute(r.Context(), wsID, HubUserIDFromContext(r.Context()), r.PostForm.Get("name"), nil)
	http.Redirect(w, r, "/account/workspaces/"+wsID, http.StatusSeeOther)
}

func (h *HubWorkspaceHandlers) addPosition(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	wsID := chi.URLParam(r, "id")
	_, _ = h.createPos.Execute(r.Context(), wsID, HubUserIDFromContext(r.Context()), r.PostForm.Get("title"))
	http.Redirect(w, r, "/account/workspaces/"+wsID, http.StatusSeeOther)
}

func (h *HubWorkspaceHandlers) enableProd(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	wsID := chi.URLParam(r, "id")
	_ = h.enableProduct.Execute(r.Context(), wsID, HubUserIDFromContext(r.Context()), r.PostForm.Get("product_key"))
	http.Redirect(w, r, "/account/workspaces/"+wsID, http.StatusSeeOther)
}
