package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
)

// SessionHandlers exposes the authenticated session-management API.
type SessionHandlers struct {
	list         *appidentity.ListSessions
	terminate    *appidentity.TerminateSession
	terminateAll *appidentity.TerminateAllSessions
}

func NewSessionHandlers(list *appidentity.ListSessions, terminate *appidentity.TerminateSession,
	terminateAll *appidentity.TerminateAllSessions) *SessionHandlers {
	return &SessionHandlers{list: list, terminate: terminate, terminateAll: terminateAll}
}

// Register mounts the /api/sessions routes (expects RequireAuth applied by caller).
func (h *SessionHandlers) Register(r chi.Router) {
	r.Get("/api/sessions", h.listSessions)
	r.Delete("/api/sessions/{id}", h.deleteSession)
	r.Post("/api/sessions/logout-all", h.logoutAll)
}

func (h *SessionHandlers) listSessions(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	sessions, err := h.list.Execute(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	type dto struct {
		ID         string `json:"id"`
		DeviceName string `json:"device_name"`
		IP         string `json:"ip"`
		Current    bool   `json:"current"`
	}
	out := make([]dto, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, dto{ID: s.ID, DeviceName: s.DeviceName, IP: s.IP})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *SessionHandlers) deleteSession(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.terminate.Execute(r.Context(), userID, id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SessionHandlers) logoutAll(w http.ResponseWriter, r *http.Request) {
	userID := UserIDFromContext(r.Context())
	if err := h.terminateAll.Execute(r.Context(), userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
