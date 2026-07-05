package identity

import (
	"context"
	"time"
)

// Token purposes for one-time tokens.
const (
	PurposeVerifyEmail   = "verify_email"
	PurposePasswordReset = "password_reset"
)

// UserRepository persists users.
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	FindByEmail(ctx context.Context, email string) (*User, error) // ErrUserNotFound if absent
	FindByID(ctx context.Context, id string) (*User, error)       // ErrUserNotFound if absent
	Update(ctx context.Context, u *User) error
}

// PasswordHasher hashes and verifies passwords.
type PasswordHasher interface {
	Hash(plain string) (string, error)
	Check(hash, plain string) bool
}

// Mailer sends transactional emails.
type Mailer interface {
	SendVerification(ctx context.Context, to, link string) error
	SendPasswordReset(ctx context.Context, to, link string) error
}

// VerificationTokens issues and consumes one-time tokens (backed by Redis + TTL).
type VerificationTokens interface {
	// Issue generates a random token bound to userID under purpose with ttl, returns the token string.
	Issue(ctx context.Context, purpose, userID string, ttl time.Duration) (string, error)
	// Consume validates and deletes the token, returning the bound userID, or ErrTokenInvalid.
	Consume(ctx context.Context, purpose, token string) (string, error)
}
