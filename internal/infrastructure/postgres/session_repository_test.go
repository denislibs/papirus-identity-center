package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

func TestSessionRepositoryCreateListEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	userRepo := NewUserRepository(w.pool)
	sessRepo := NewSessionRepository(w.pool)

	// a session references a user (FK), so create the user first
	uid := "33333333-3333-3333-3333-333333333333"
	require.NoError(t, userRepo.Create(ctx, &identity.User{
		ID: uid, Email: "s@example.com", PasswordHash: "h", Locale: "en", Timezone: "UTC", CreatedAt: time.Now().UTC(),
	}))

	s := &identity.Session{
		ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", UserID: uid, HydraSessionID: "sid-1",
		DeviceName: "Chrome on Mac", UserAgent: "UA", IP: "1.2.3.4",
		CreatedAt: time.Now().UTC(), LastSeenAt: time.Now().UTC(),
	}
	require.NoError(t, sessRepo.Create(ctx, s))

	active, err := sessRepo.ListActiveByUser(ctx, uid)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, "sid-1", active[0].HydraSessionID)

	require.NoError(t, sessRepo.MarkEnded(ctx, s.ID))
	active, err = sessRepo.ListActiveByUser(ctx, uid)
	require.NoError(t, err)
	require.Len(t, active, 0)

	_, err = sessRepo.FindByID(ctx, "no-such-id")
	require.ErrorIs(t, err, identity.ErrSessionNotFound)
}

func TestSessionRepositoryEndByHydraSIDAndAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx, w := newMigratedPool(t)
	userRepo := NewUserRepository(w.pool)
	sessRepo := NewSessionRepository(w.pool)

	uid := "44444444-4444-4444-4444-444444444444"
	require.NoError(t, userRepo.Create(ctx, &identity.User{
		ID: uid, Email: "m@example.com", PasswordHash: "h", Locale: "en", Timezone: "UTC", CreatedAt: time.Now().UTC(),
	}))
	mk := func(id, sid string) *identity.Session {
		return &identity.Session{ID: id, UserID: uid, HydraSessionID: sid, CreatedAt: time.Now().UTC(), LastSeenAt: time.Now().UTC()}
	}
	require.NoError(t, sessRepo.Create(ctx, mk("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "sid-A")))
	require.NoError(t, sessRepo.Create(ctx, mk("cccccccc-cccc-cccc-cccc-cccccccccccc", "sid-B")))

	require.NoError(t, sessRepo.MarkEndedByHydraSID(ctx, "sid-A"))
	active, _ := sessRepo.ListActiveByUser(ctx, uid)
	require.Len(t, active, 1)

	require.NoError(t, sessRepo.MarkAllEndedByUser(ctx, uid))
	active, _ = sessRepo.ListActiveByUser(ctx, uid)
	require.Len(t, active, 0)
}
