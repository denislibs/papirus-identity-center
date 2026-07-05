package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestConnectPings(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("platform"),
		tcpostgres.WithUsername("platform"),
		tcpostgres.WithPassword("platform"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
	)
	require.NoError(t, err)
	defer func() { _ = container.Terminate(ctx) }()

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	require.NoError(t, pool.Ping(ctx))
}
