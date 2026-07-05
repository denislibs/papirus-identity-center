package identity_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

func TestRegisterUserCreatesUnverifiedAndSendsMail(t *testing.T) {
	users := newFakeUsers()
	tokens := newFakeTokens()
	mailer := newFakeMailer()
	uc := identity.NewRegisterUser(users, &fakeHasher{}, tokens, mailer, "https://acc.example")

	u, err := uc.Execute(context.Background(), identity.RegisterInput{
		Email: "  Alice@Example.com ", Password: "long-enough-pw", Name: "Alice",
	})
	require.NoError(t, err)
	require.NotEmpty(t, u.ID)
	require.Equal(t, "alice@example.com", u.Email) // normalized
	require.False(t, u.EmailVerified)
	require.Equal(t, "hashed:long-enough-pw", u.PasswordHash)

	// user persisted
	stored, err := users.FindByEmail(context.Background(), "alice@example.com")
	require.NoError(t, err)
	require.Equal(t, u.ID, stored.ID)

	// verification mail sent with a link containing the issued token
	require.Len(t, mailer.verifications, 1)
	require.Equal(t, "alice@example.com", mailer.verifications[0].to)
	require.True(t, strings.Contains(mailer.verifications[0].link, tokens.lastToken))
}

func TestRegisterUserRejectsDuplicate(t *testing.T) {
	users := newFakeUsers()
	uc := identity.NewRegisterUser(users, &fakeHasher{}, newFakeTokens(), newFakeMailer(), "https://acc.example")
	_, err := uc.Execute(context.Background(), identity.RegisterInput{Email: "a@x.com", Password: "long-enough-pw"})
	require.NoError(t, err)
	_, err = uc.Execute(context.Background(), identity.RegisterInput{Email: "a@x.com", Password: "long-enough-pw"})
	require.ErrorIs(t, err, domain.ErrUserExists)
}

func TestRegisterUserRejectsWeakPassword(t *testing.T) {
	uc := identity.NewRegisterUser(newFakeUsers(), &fakeHasher{}, newFakeTokens(), newFakeMailer(), "https://acc.example")
	_, err := uc.Execute(context.Background(), identity.RegisterInput{Email: "a@x.com", Password: "short"})
	require.ErrorIs(t, err, domain.ErrWeakPassword)
}

func TestRegisterUserRejectsEmptyEmail(t *testing.T) {
	uc := identity.NewRegisterUser(newFakeUsers(), &fakeHasher{}, newFakeTokens(), newFakeMailer(), "https://acc.example")
	_, err := uc.Execute(context.Background(), identity.RegisterInput{Email: "  ", Password: "long-enough-pw"})
	require.ErrorIs(t, err, domain.ErrInvalidEmail)
}
