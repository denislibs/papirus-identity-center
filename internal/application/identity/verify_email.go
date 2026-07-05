package identity

import (
	"context"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// VerifyEmail consumes a verification token and marks the user's email verified.
type VerifyEmail struct {
	users  domain.UserRepository
	tokens domain.VerificationTokens
}

func NewVerifyEmail(users domain.UserRepository, tokens domain.VerificationTokens) *VerifyEmail {
	return &VerifyEmail{users: users, tokens: tokens}
}

func (uc *VerifyEmail) Execute(ctx context.Context, token string) error {
	userID, err := uc.tokens.Consume(ctx, domain.PurposeVerifyEmail, token)
	if err != nil {
		return err // ErrTokenInvalid
	}
	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	u.EmailVerified = true
	return uc.users.Update(ctx, u)
}
