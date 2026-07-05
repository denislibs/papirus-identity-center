package mail

import (
	"bytes"
	"context"
	"log"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogMailerLogsLink(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	m := NewLogMailer(logger)

	require.NoError(t, m.SendVerification(context.Background(), "a@example.com", "https://x/verify?token=abc"))

	out := buf.String()
	require.Contains(t, out, "a@example.com")
	require.Contains(t, out, "https://x/verify?token=abc")
}
