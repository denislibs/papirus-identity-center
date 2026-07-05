package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/papyrus/platform/internal/domain/identity"
)

type pgxpoolWrapper struct{ pool *pgxpool.Pool }

func newMigratedPool(t *testing.T) (context.Context, *pgxpoolWrapper) {
	t.Helper()
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
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, RunMigrations(dsn))

	pool, err := Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return ctx, &pgxpoolWrapper{pool}
}

func TestUserRepositoryCreateAndFind(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	repo := NewUserRepository(w.pool)

	u := &identity.User{
		ID: "11111111-1111-1111-1111-111111111111", Email: "a@example.com",
		PasswordHash: "hash", Name: "Alice", Locale: "en", Timezone: "UTC",
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, u))

	byEmail, err := repo.FindByEmail(ctx, "a@example.com")
	require.NoError(t, err)
	require.Equal(t, u.ID, byEmail.ID)
	require.False(t, byEmail.EmailVerified)

	byID, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "a@example.com", byID.Email)

	_, err = repo.FindByEmail(ctx, "missing@example.com")
	require.ErrorIs(t, err, identity.ErrUserNotFound)
}

func TestUserRepositoryUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	repo := NewUserRepository(w.pool)

	u := &identity.User{
		ID: "22222222-2222-2222-2222-222222222222", Email: "b@example.com",
		PasswordHash: "h", Locale: "en", Timezone: "UTC", CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, u))

	u.EmailVerified = true
	u.PasswordHash = "newhash"
	require.NoError(t, repo.Update(ctx, u))

	got, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	require.True(t, got.EmailVerified)
	require.Equal(t, "newhash", got.PasswordHash)
}
