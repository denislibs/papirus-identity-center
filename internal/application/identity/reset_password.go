package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

const resetTokenTTL = 1 * time.Hour

// RequestPasswordReset issues a reset token and emails a link. It never reveals
// whether the email exists (no account enumeration).
type RequestPasswordReset struct {
	users   domain.UserRepository
	tokens  domain.VerificationTokens
	mailer  domain.Mailer
	baseURL string
}

func NewRequestPasswordReset(users domain.UserRepository, tokens domain.VerificationTokens,
	mailer domain.Mailer, baseURL string) *RequestPasswordReset {
	return &RequestPasswordReset{users: users, tokens: tokens, mailer: mailer, baseURL: baseURL}
}

func (uc *RequestPasswordReset) Execute(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := uc.users.FindByEmail(ctx, email)
	if errors.Is(err, domain.ErrUserNotFound) {
		return nil // silent: do not reveal absence
	}
	if err != nil {
		return err
	}
	token, err := uc.tokens.Issue(ctx, domain.PurposePasswordReset, u.ID, resetTokenTTL)
	if err != nil {
		return err
	}
	link := uc.baseURL + "/reset-password?token=" + token
	return uc.mailer.SendPasswordReset(ctx, u.Email, link)
}

// ResetPassword consumes a reset token and sets a new password hash.
type ResetPassword struct {
	users  domain.UserRepository
	hasher domain.PasswordHasher
	tokens domain.VerificationTokens
}

func NewResetPassword(users domain.UserRepository, hasher domain.PasswordHasher,
	tokens domain.VerificationTokens) *ResetPassword {
	return &ResetPassword{users: users, hasher: hasher, tokens: tokens}
}

func (uc *ResetPassword) Execute(ctx context.Context, token, newPassword string) error {
	if len(newPassword) < minPasswordLen {
		return domain.ErrWeakPassword
	}
	userID, err := uc.tokens.Consume(ctx, domain.PurposePasswordReset, token)
	if err != nil {
		return err // ErrTokenInvalid
	}
	u, err := uc.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	hash, err := uc.hasher.Hash(newPassword)
	if err != nil {
		return err
	}
	u.PasswordHash = hash
	return uc.users.Update(ctx, u)
}
