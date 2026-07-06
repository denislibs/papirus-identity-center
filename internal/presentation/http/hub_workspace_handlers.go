package http

import (
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	appws "github.com/denislibs/papirus-identity-center/internal/application/workspace"
)

// HubWorkspaceHandlers renders workspace management pages in the account hub.
type HubWorkspaceHandlers struct {
	create        *appws.CreateWorkspace
	listMine      *appws.ListMyWorkspaces
	listMembers   *appws.ListMembers
	invite        *appws.InviteMember
	createUnit    *appws.CreateOrgUnit
	listUnits     *appws.ListOrgUnits
	createPos     *appws.CreatePosition
	listPos       *appws.ListPositions
	listProducts  *appws.ListProducts
	enableProduct *appws.EnableProduct
	listEnabled   *appws.ListEnabledProducts
	tpl           *template.Template
}

func NewHubWorkspaceHandlers(create *appws.CreateWorkspace, listMine *appws.ListMyWorkspaces,
	listMembers *appws.ListMembers, invite *appws.InviteMember, createUnit *appws.CreateOrgUnit,
	listUnits *appws.ListOrgUnits, createPos *appws.CreatePosition, listPos *appws.ListPositions,
	listProducts *appws.ListProducts, enableProduct *appws.EnableProduct, listEnabled *appws.ListEnabledProducts,
	tpl *template.Template) *HubWorkspaceHandlers {
	return &HubWorkspaceHandlers{
		create: create, listMine: listMine, listMembers: listMembers, invite: invite,
		createUnit: createUnit, listUnits: listUnits, createPos: createPos, listPos: listPos,
		listProducts: listProducts, enableProduct: enableProduct, listEnabled: listEnabled, tpl: tpl,
	}
}

func (h *HubWorkspaceHandlers) Register(r chi.Router) {
	r.Get("/account/workspaces", h.list)
	r.Post("/account/workspaces", h.createWorkspace)
	// detail + management routes
	h.registerDetail(r)
}

func (h *HubWorkspaceHandlers) list(w http.ResponseWriter, r *http.Request) {
	userID := HubUserIDFromContext(r.Context())
	list, err := h.listMine.Execute(r.Context(), userID)
	if err != nil {
		http.Error(w, "workspaces error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.tpl.ExecuteTemplate(w, "workspaces.html", map[string]any{"Workspaces": list})
}

func (h *HubWorkspaceHandlers) createWorkspace(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	userID := HubUserIDFromContext(r.Context())
	ws, err := h.create.Execute(r.Context(), userID, r.PostForm.Get("name"))
	if err != nil {
		http.Error(w, "could not create workspace", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/account/workspaces/"+ws.ID, http.StatusSeeOther)
}
