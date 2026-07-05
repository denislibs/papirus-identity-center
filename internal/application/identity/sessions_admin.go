package identity

import (
	"context"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

// ListSessions returns a user's active sessions.
type ListSessions struct {
	sessions domain.SessionRepository
}

func NewListSessions(sessions domain.SessionRepository) *ListSessions {
	return &ListSessions{sessions: sessions}
}

func (uc *ListSessions) Execute(ctx context.Context, userID string) ([]*domain.Session, error) {
	return uc.sessions.ListActiveByUser(ctx, userID)
}

// TerminateSession ends one of the user's own sessions (revokes it in Hydra too).
type TerminateSession struct {
	sessions domain.SessionRepository
	hydra    domain.HydraClient
}

func NewTerminateSession(sessions domain.SessionRepository, hydra domain.HydraClient) *TerminateSession {
	return &TerminateSession{sessions: sessions, hydra: hydra}
}

func (uc *TerminateSession) Execute(ctx context.Context, userID, sessionID string) error {
	s, err := uc.sessions.FindByID(ctx, sessionID)
	if err != nil {
		return err // ErrSessionNotFound
	}
	if s.UserID != userID {
		return domain.ErrSessionNotFound // ownership: don't reveal others' sessions
	}
	if s.HydraSessionID != "" {
		if err := uc.hydra.RevokeLoginSessionByID(ctx, s.HydraSessionID); err != nil {
			return err
		}
	}
	return uc.sessions.MarkEnded(ctx, sessionID)
}

// TerminateAllSessions ends every session of the user ("logout everywhere").
type TerminateAllSessions struct {
	sessions domain.SessionRepository
	hydra    domain.HydraClient
}

func NewTerminateAllSessions(sessions domain.SessionRepository, hydra domain.HydraClient) *TerminateAllSessions {
	return &TerminateAllSessions{sessions: sessions, hydra: hydra}
}

func (uc *TerminateAllSessions) Execute(ctx context.Context, userID string) error {
	if err := uc.hydra.RevokeLoginSessionsBySubject(ctx, userID); err != nil {
		return err
	}
	return uc.sessions.MarkAllEndedByUser(ctx, userID)
}
