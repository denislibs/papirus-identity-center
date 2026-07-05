package identity_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

func TestVerifyEmailMarksVerified(t *testing.T) {
	users := newFakeUsers()
	tokens := newFakeTokens()
	// seed an unverified user + a token
	u := &domain.User{ID: "u1", Email: "a@x.com", EmailVerified: false}
	_ = users.Create(context.Background(), u)
	tok, _ := tokens.Issue(context.Background(), domain.PurposeVerifyEmail, "u1", 0)

	uc := identity.NewVerifyEmail(users, tokens)
	require.NoError(t, uc.Execute(context.Background(), tok))

	got, _ := users.FindByID(context.Background(), "u1")
	require.True(t, got.EmailVerified)
}

func TestVerifyEmailRejectsBadToken(t *testing.T) {
	uc := identity.NewVerifyEmail(newFakeUsers(), newFakeTokens())
	err := uc.Execute(context.Background(), "nope")
	require.ErrorIs(t, err, domain.ErrTokenInvalid)
}
