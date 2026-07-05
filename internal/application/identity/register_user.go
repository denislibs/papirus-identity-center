package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

const verifyTokenTTL = 24 * time.Hour
const minPasswordLen = 8

// RegisterInput is the request to register a new account.
type RegisterInput struct {
	Email    string
	Password string
	Name     string
	Locale   string
	Timezone string
}

// RegisterUser creates an unverified account and emails a verification link.
type RegisterUser struct {
	users   domain.UserRepository
	hasher  domain.PasswordHasher
	tokens  domain.VerificationTokens
	mailer  domain.Mailer
	baseURL string
}

func NewRegisterUser(users domain.UserRepository, hasher domain.PasswordHasher,
	tokens domain.VerificationTokens, mailer domain.Mailer, baseURL string) *RegisterUser {
	return &RegisterUser{users: users, hasher: hasher, tokens: tokens, mailer: mailer, baseURL: baseURL}
}

func (uc *RegisterUser) Execute(ctx context.Context, in RegisterInput) (*domain.User, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, domain.ErrInvalidEmail
	}
	if len(in.Password) < minPasswordLen {
		return nil, domain.ErrWeakPassword
	}

	if _, err := uc.users.FindByEmail(ctx, email); err == nil {
		return nil, domain.ErrUserExists
	} else if !errors.Is(err, domain.ErrUserNotFound) {
		return nil, err
	}

	hash, err := uc.hasher.Hash(in.Password)
	if err != nil {
		return nil, err
	}

	locale := in.Locale
	if locale == "" {
		locale = "en"
	}
	tz := in.Timezone
	if tz == "" {
		tz = "UTC"
	}

	u := &domain.User{
		ID: uuid.NewString(), Email: email, EmailVerified: false, PasswordHash: hash,
		Name: strings.TrimSpace(in.Name), Locale: locale, Timezone: tz, CreatedAt: time.Now().UTC(),
	}
	if err := uc.users.Create(ctx, u); err != nil {
		return nil, err
	}

	token, err := uc.tokens.Issue(ctx, domain.PurposeVerifyEmail, u.ID, verifyTokenTTL)
	if err != nil {
		return nil, err
	}
	link := uc.baseURL + "/verify-email?token=" + token
	if err := uc.mailer.SendVerification(ctx, u.Email, link); err != nil {
		return nil, err
	}
	return u, nil
}
