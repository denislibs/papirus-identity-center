package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

func TestHubSessionStore(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()
	container, err := tcredis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	defer func() { _ = container.Terminate(ctx) }()
	endpoint, err := container.Endpoint(ctx, "")
	require.NoError(t, err)
	client, err := Connect(ctx, endpoint)
	require.NoError(t, err)
	defer client.Close()

	store := NewHubSessionStore(client, time.Hour)

	id, err := store.Create(ctx, "user-1")
	require.NoError(t, err)
	require.NotEmpty(t, id)

	sub, err := store.Subject(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "user-1", sub)

	require.NoError(t, store.Delete(ctx, id))
	_, err = store.Subject(ctx, id)
	require.ErrorIs(t, err, identity.ErrSessionNotFound)
}
