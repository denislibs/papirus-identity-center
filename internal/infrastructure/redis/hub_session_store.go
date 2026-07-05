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

// HubSessionStore implements identity.HubSessionStore backed by Redis with TTL.
type HubSessionStore struct {
	client *goredis.Client
	ttl    time.Duration
}

func NewHubSessionStore(client *goredis.Client, ttl time.Duration) *HubSessionStore {
	return &HubSessionStore{client: client, ttl: ttl}
}

func hubKey(id string) string { return "hubsession:" + id }

func (s *HubSessionStore) Create(ctx context.Context, subject string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("redis: gen hub session id: %w", err)
	}
	id := hex.EncodeToString(buf)
	if err := s.client.Set(ctx, hubKey(id), subject, s.ttl).Err(); err != nil {
		return "", fmt.Errorf("redis: store hub session: %w", err)
	}
	return id, nil
}

func (s *HubSessionStore) Subject(ctx context.Context, id string) (string, error) {
	sub, err := s.client.Get(ctx, hubKey(id)).Result()
	if errors.Is(err, goredis.Nil) {
		return "", identity.ErrSessionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("redis: get hub session: %w", err)
	}
	return sub, nil
}

func (s *HubSessionStore) Delete(ctx context.Context, id string) error {
	if err := s.client.Del(ctx, hubKey(id)).Err(); err != nil {
		return fmt.Errorf("redis: delete hub session: %w", err)
	}
	return nil
}
