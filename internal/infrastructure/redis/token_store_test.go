package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/papyrus/platform/internal/domain/identity"
)

func TestTokenStoreIssueConsume(t *testing.T) {
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

	store := NewTokenStore(client)

	token, err := store.Issue(ctx, identity.PurposeVerifyEmail, "user-1", time.Minute)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	userID, err := store.Consume(ctx, identity.PurposeVerifyEmail, token)
	require.NoError(t, err)
	require.Equal(t, "user-1", userID)

	// second consume must fail (single use)
	_, err = store.Consume(ctx, identity.PurposeVerifyEmail, token)
	require.ErrorIs(t, err, identity.ErrTokenInvalid)

	// wrong purpose must fail
	token2, err := store.Issue(ctx, identity.PurposePasswordReset, "user-2", time.Minute)
	require.NoError(t, err)
	_, err = store.Consume(ctx, identity.PurposeVerifyEmail, token2)
	require.ErrorIs(t, err, identity.ErrTokenInvalid)
}
