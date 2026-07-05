package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	appws "github.com/denislibs/papirus-identity-center/internal/application/workspace"
	domainws "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

// WorkspaceHandlers provides REST endpoints for workspaces, members and invites.
type WorkspaceHandlers struct {
	create   *appws.CreateWorkspace
	listMine *appws.ListMyWorkspaces
	members  *appws.ListMembers
	invite   *appws.InviteMember
	accept   *appws.AcceptInvite
}

func NewWorkspaceHandlers(create *appws.CreateWorkspace, listMine *appws.ListMyWorkspaces,
	members *appws.ListMembers, invite *appws.InviteMember, accept *appws.AcceptInvite) *WorkspaceHandlers {
	return &WorkspaceHandlers{create: create, listMine: listMine, members: members, invite: invite, accept: accept}
}

// Register mounts workspace routes on r. All routes expect RequireAuth applied by the caller.
func (h *WorkspaceHandlers) Register(r chi.Router) {
	r.Post("/workspaces", h.createWorkspace)
	r.Get("/me/workspaces", h.listMyWorkspaces)
	r.Get("/workspaces/{id}/members", h.listMembers)
	r.Post("/workspaces/{id}/invites", h.inviteMember)
	r.Post("/invites/{token}/accept", h.acceptInvite)
}

// wsErr maps workspace domain errors to HTTP responses.
func wsErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domainws.ErrInvalidName):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid name"})
	case errors.Is(err, domainws.ErrInvalidRole):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role"})
	case errors.Is(err, domainws.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
	case errors.Is(err, domainws.ErrNotMember):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member"})
	case errors.Is(err, domainws.ErrAlreadyMember):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already a member"})
	case errors.Is(err, domainws.ErrWorkspaceNotFound), errors.Is(err, domainws.ErrInviteNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (h *WorkspaceHandlers) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var body struct{ Name string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	ws, err := h.create.Execute(r.Context(), UserIDFromContext(r.Context()), body.Name)
	if err != nil {
		wsErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": ws.ID, "slug": ws.Slug})
}

func (h *WorkspaceHandlers) listMyWorkspaces(w http.ResponseWriter, r *http.Request) {
	list, err := h.listMine.Execute(r.Context(), UserIDFromContext(r.Context()))
	if err != nil {
		wsErr(w, err)
		return
	}
	type dto struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	out := make([]dto, 0, len(list))
	for _, ws := range list {
		out = append(out, dto{ws.ID, ws.Name, ws.Slug})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *WorkspaceHandlers) listMembers(w http.ResponseWriter, r *http.Request) {
	list, err := h.members.Execute(r.Context(), chi.URLParam(r, "id"), UserIDFromContext(r.Context()))
	if err != nil {
		wsErr(w, err)
		return
	}
	type dto struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
		Status string `json:"status"`
	}
	out := make([]dto, 0, len(list))
	for _, m := range list {
		out = append(out, dto{m.UserID, m.Role, m.Status})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *WorkspaceHandlers) inviteMember(w http.ResponseWriter, r *http.Request) {
	var body struct{ Email, Role string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	err := h.invite.Execute(r.Context(), chi.URLParam(r, "id"), UserIDFromContext(r.Context()), body.Email, body.Role)
	if err != nil {
		wsErr(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "invited"})
}

func (h *WorkspaceHandlers) acceptInvite(w http.ResponseWriter, r *http.Request) {
	if err := h.accept.Execute(r.Context(), chi.URLParam(r, "token"), UserIDFromContext(r.Context())); err != nil {
		wsErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
