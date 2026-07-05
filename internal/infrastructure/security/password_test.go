package security

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHashAndCheck(t *testing.T) {
	h := NewBcryptHasher(0) // 0 → default cost

	hash, err := h.Hash("s3cret-password")
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	require.NotEqual(t, "s3cret-password", hash)

	require.True(t, h.Check(hash, "s3cret-password"))
	require.False(t, h.Check(hash, "wrong-password"))
}
