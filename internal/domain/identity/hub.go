package identity

import "context"

// CodeExchanger exchanges an OAuth2 authorization code for the authenticated
// user's subject (which equals the platform user id).
type CodeExchanger interface {
	ExchangeForSubject(ctx context.Context, code string) (subject string, err error)
}

// HubSessionStore persists server-side hub browser sessions (keyed by opaque id).
type HubSessionStore interface {
	// Create stores a new session for the subject, returns the opaque session id.
	Create(ctx context.Context, subject string) (id string, err error)
	// Subject returns the subject bound to the session id, or ErrSessionNotFound.
	Subject(ctx context.Context, id string) (string, error)
	// Delete removes a session (logout).
	Delete(ctx context.Context, id string) error
}
