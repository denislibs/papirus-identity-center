package http

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderLogin(t *testing.T) {
	tpl := MustLoadTemplates()
	var buf bytes.Buffer
	require.NoError(t, tpl.ExecuteTemplate(&buf, "login.html", map[string]any{
		"Challenge": "chal-123", "Error": "",
	}))
	out := buf.String()
	require.True(t, strings.Contains(out, `value="chal-123"`))
	require.True(t, strings.Contains(out, `action="/login"`))
}
