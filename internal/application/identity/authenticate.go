package identity

import (
	"context"
	"errors"
	"strings"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// Authenticate verifies an email/password pair and returns the user.
type Authenticate struct {
	users  domain.UserRepository
	hasher domain.PasswordHasher
}

func NewAuthenticate(users domain.UserRepository, hasher domain.PasswordHasher) *Authenticate {
	return &Authenticate{users: users, hasher: hasher}
}

func (uc *Authenticate) Execute(ctx context.Context, email, password string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := uc.users.FindByEmail(ctx, email)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil, domain.ErrInvalidCredentials // do not reveal which part failed
	}
	if err != nil {
		return nil, err
	}
	if !uc.hasher.Check(u.PasswordHash, password) {
		return nil, domain.ErrInvalidCredentials
	}
	if !u.EmailVerified {
		return nil, domain.ErrEmailNotVerified
	}
	return u, nil
}
