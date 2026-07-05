package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// TokenStore implements identity.VerificationTokens using Redis with TTL.
type TokenStore struct {
	client *goredis.Client
}

func NewTokenStore(client *goredis.Client) *TokenStore {
	return &TokenStore{client: client}
}

func key(purpose, token string) string {
	return fmt.Sprintf("token:%s:%s", purpose, token)
}

func (s *TokenStore) Issue(ctx context.Context, purpose, userID string, ttl time.Duration) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("redis: generate token: %w", err)
	}
	token := hex.EncodeToString(buf)
	if err := s.client.Set(ctx, key(purpose, token), userID, ttl).Err(); err != nil {
		return "", fmt.Errorf("redis: store token: %w", err)
	}
	return token, nil
}

func (s *TokenStore) Consume(ctx context.Context, purpose, token string) (string, error) {
	// GETDEL: atomic get + delete (single-use).
	userID, err := s.client.GetDel(ctx, key(purpose, token)).Result()
	if errors.Is(err, goredis.Nil) {
		return "", identity.ErrTokenInvalid
	}
	if err != nil {
		return "", fmt.Errorf("redis: consume token: %w", err)
	}
	return userID, nil
}
