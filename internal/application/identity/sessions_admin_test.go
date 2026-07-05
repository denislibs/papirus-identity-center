package identity_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/denislibs/papirus-identity-center/internal/application/identity"
	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

func TestListSessions(t *testing.T) {
	sessions := newFakeSessionRepo()
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "u1", HydraSessionID: "sid1"})
	uc := identity.NewListSessions(sessions)

	got, err := uc.Execute(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "s1", got[0].ID)
}

func TestTerminateSessionRevokesAndEnds(t *testing.T) {
	sessions := newFakeSessionRepo()
	hydra := &fakeHydraAdmin{}
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "u1", HydraSessionID: "sid1"})
	uc := identity.NewTerminateSession(sessions, hydra)

	require.NoError(t, uc.Execute(context.Background(), "u1", "s1"))
	require.Equal(t, "sid1", hydra.revokedSID)
	require.True(t, sessions.ended["s1"])
}

func TestTerminateSessionRejectsOtherUsersSession(t *testing.T) {
	sessions := newFakeSessionRepo()
	hydra := &fakeHydraAdmin{}
	_ = sessions.Create(context.Background(), &domain.Session{ID: "s1", UserID: "OWNER", HydraSessionID: "sid1"})
	uc := identity.NewTerminateSession(sessions, hydra)

	err := uc.Execute(context.Background(), "ATTACKER", "s1")
	require.ErrorIs(t, err, domain.ErrSessionNotFound)
	require.Empty(t, hydra.revokedSID)    // no revoke
	require.False(t, sessions.ended["s1"]) // not ended
}

func TestTerminateAllSessions(t *testing.T) {
	sessions := newFakeSessionRepo()
	hydra := &fakeHydraAdmin{}
	uc := identity.NewTerminateAllSessions(sessions, hydra)

	require.NoError(t, uc.Execute(context.Background(), "u1"))
	require.Equal(t, "u1", hydra.revokedSubject)
	require.True(t, sessions.allEnded["u1"])
}
