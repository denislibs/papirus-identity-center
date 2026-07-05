package identity

import (
	"context"
	"time"
)

// Session is a rich record of an authenticated browser session, linked to a
// Hydra login session (HydraSessionID = Hydra "sid").
type Session struct {
	ID             string
	UserID         string
	HydraSessionID string
	DeviceName     string
	UserAgent      string
	IP             string
	Location       string
	CreatedAt      time.Time
	LastSeenAt     time.Time
	EndedAt        *time.Time
}

// SessionRepository persists sessions.
type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	FindByID(ctx context.Context, id string) (*Session, error) // ErrSessionNotFound if absent
	ListActiveByUser(ctx context.Context, userID string) ([]*Session, error)
	MarkEnded(ctx context.Context, id string) error
	MarkEndedByHydraSID(ctx context.Context, sid string) error
	MarkAllEndedByUser(ctx context.Context, userID string) error
}
