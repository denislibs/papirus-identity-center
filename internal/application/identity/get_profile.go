package identity

import (
	"context"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// GetProfile fetches a user's profile by id.
type GetProfile struct {
	users domain.UserRepository
}

func NewGetProfile(users domain.UserRepository) *GetProfile {
	return &GetProfile{users: users}
}

func (uc *GetProfile) Execute(ctx context.Context, userID string) (*domain.User, error) {
	return uc.users.FindByID(ctx, userID)
}
