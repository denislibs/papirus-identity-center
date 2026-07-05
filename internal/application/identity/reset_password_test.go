package identity_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

func TestRequestPasswordResetSendsMailForExistingUser(t *testing.T) {
	users := newFakeUsers()
	tokens := newFakeTokens()
	mailer := newFakeMailer()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com"})

	uc := identity.NewRequestPasswordReset(users, tokens, mailer, "https://acc.example")
	require.NoError(t, uc.Execute(context.Background(), "A@x.com")) // case-insensitive

	require.Len(t, mailer.resets, 1)
	require.Equal(t, "a@x.com", mailer.resets[0].to)
	require.True(t, strings.Contains(mailer.resets[0].link, tokens.lastToken))
}

func TestRequestPasswordResetSilentForUnknownUser(t *testing.T) {
	mailer := newFakeMailer()
	uc := identity.NewRequestPasswordReset(newFakeUsers(), newFakeTokens(), mailer, "https://acc.example")
	// must NOT error (no account enumeration) and must NOT send mail
	require.NoError(t, uc.Execute(context.Background(), "ghost@x.com"))
	require.Len(t, mailer.resets, 0)
}

func TestResetPasswordSetsNewHash(t *testing.T) {
	users := newFakeUsers()
	tokens := newFakeTokens()
	_ = users.Create(context.Background(), &domain.User{ID: "u1", Email: "a@x.com", PasswordHash: "old"})
	tok, _ := tokens.Issue(context.Background(), domain.PurposePasswordReset, "u1", 0)

	uc := identity.NewResetPassword(users, &fakeHasher{}, tokens)
	require.NoError(t, uc.Execute(context.Background(), tok, "brand-new-pw"))

	got, _ := users.FindByID(context.Background(), "u1")
	require.Equal(t, "hashed:brand-new-pw", got.PasswordHash)
}

func TestResetPasswordRejectsWeak(t *testing.T) {
	uc := identity.NewResetPassword(newFakeUsers(), &fakeHasher{}, newFakeTokens())
	err := uc.Execute(context.Background(), "any", "short")
	require.ErrorIs(t, err, domain.ErrWeakPassword)
}

func TestResetPasswordRejectsBadToken(t *testing.T) {
	uc := identity.NewResetPassword(newFakeUsers(), &fakeHasher{}, newFakeTokens())
	err := uc.Execute(context.Background(), "nope", "brand-new-pw")
	require.ErrorIs(t, err, domain.ErrTokenInvalid)
}
