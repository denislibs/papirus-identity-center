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

func TestRunMigrationsCreatesSessions(t *testing.T) {
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
	require.NoError(t, RunMigrations(dsn))

	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	var exists bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name='sessions')`).Scan(&exists))
	require.True(t, exists)
}

func TestRunMigrationsCreatesWorkspaces(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("platform"), tcpostgres.WithUsername("platform"), tcpostgres.WithPassword("platform"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	require.NoError(t, err)
	defer func() { _ = container.Terminate(ctx) }()
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, RunMigrations(dsn))
	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	for _, tbl := range []string{"workspaces", "workspace_members", "workspace_invites"} {
		var exists bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name=$1)`, tbl).Scan(&exists))
		require.True(t, exists, tbl)
	}
}

func TestRunMigrationsCreatesOrgStructure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("platform"), tcpostgres.WithUsername("platform"), tcpostgres.WithPassword("platform"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	require.NoError(t, err)
	defer func() { _ = container.Terminate(ctx) }()
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, RunMigrations(dsn))
	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	for _, tbl := range []string{"org_units", "positions"} {
		var ok bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name=$1)`, tbl).Scan(&ok))
		require.True(t, ok, tbl)
	}
	for _, col := range []string{"org_unit_id", "position_id"} {
		var ok bool
		require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.columns WHERE table_name='workspace_members' AND column_name=$1)`, col).Scan(&ok))
		require.True(t, ok, col)
	}
}

func TestRunMigrationsCreatesProducts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("platform"), tcpostgres.WithUsername("platform"), tcpostgres.WithPassword("platform"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(60*time.Second)))
	require.NoError(t, err)
	defer func() { _ = container.Terminate(ctx) }()
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, RunMigrations(dsn))
	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM products`).Scan(&n))
	require.GreaterOrEqual(t, n, 2)
	var ok bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name='workspace_products')`).Scan(&ok))
	require.True(t, ok)
}

func TestRunMigrationsCreatesUsers(t *testing.T) {
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

	require.NoError(t, RunMigrations(dsn))

	// running again must be a no-op (idempotent)
	require.NoError(t, RunMigrations(dsn))

	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()

	var exists bool
	err = pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'users')`).
		Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists)
}
