package redis

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func TestConnectPings(t *testing.T) {
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

	require.NoError(t, client.Ping(ctx).Err())
}
