package identity

import "errors"

var (
	// ErrUserNotFound is returned by UserRepository lookups when no row matches.
	ErrUserNotFound = errors.New("identity: user not found")
	// ErrUserExists is returned when registering an email that already exists.
	ErrUserExists = errors.New("identity: user already exists")
	// ErrTokenInvalid is returned when a one-time token is missing or expired.
	ErrTokenInvalid = errors.New("identity: token invalid or expired")
	// ErrWeakPassword is returned when a password does not meet policy.
	ErrWeakPassword = errors.New("identity: password too weak")
	// ErrInvalidEmail is returned when an email is empty/malformed.
	ErrInvalidEmail = errors.New("identity: invalid email")
)
