package identity_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/papyrus/platform/internal/application/identity"
	domain "github.com/papyrus/platform/internal/domain/identity"
)

func TestAuthenticateSuccess(t *testing.T) {
	users := newFakeUsers()
	_ = users.Create(context.Background(), &domain.User{
		ID: "u1", Email: "a@x.com", PasswordHash: "hashed:pw", EmailVerified: true,
	})
	uc := identity.NewAuthenticate(users, fakeHasher{})

	u, err := uc.Execute(context.Background(), "A@x.com", "pw") // email case-insensitive
	require.NoError(t, err)
	require.Equal(t, "u1", u.ID)
}

func TestAuthenticateWrongPassword(t *testing.T) {
	users := newFakeUsers()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "hashed:pw", EmailVerified: true})
	uc := identity.NewAuthenticate(users, fakeHasher{})
	_, err := uc.Execute(context.Background(), "a@x.com", "wrong")
	require.ErrorIs(t, err, domain.ErrInvalidCredentials)
}

func TestAuthenticateUnknownUserIsInvalidCredentials(t *testing.T) {
	uc := identity.NewAuthenticate(newFakeUsers(), fakeHasher{})
	_, err := uc.Execute(context.Background(), "ghost@x.com", "pw")
	require.ErrorIs(t, err, domain.ErrInvalidCredentials) // NOT ErrUserNotFound (no enumeration)
}

func TestAuthenticateUnverifiedEmail(t *testing.T) {
	users := newFakeUsers()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "hashed:pw", EmailVerified: false})
	uc := identity.NewAuthenticate(users, fakeHasher{})
	_, err := uc.Execute(context.Background(), "a@x.com", "pw")
	require.ErrorIs(t, err, domain.ErrEmailNotVerified)
}
