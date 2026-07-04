package slog

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpandEscapedWhitespaceInJSONStrings(t *testing.T) {
	t.Parallel()

	input := []byte(`{
  "msg": "database query",
  "sql": "SELECT 1;\nSELECT 2;\tDONE"
}`)

	got := expandEscapedWhitespaceInJSONStrings(input)
	require.Contains(t, string(got), "SELECT 1;\nSELECT 2;\tDONE")
	require.NotContains(t, string(got), `\n`)
	require.NotContains(t, string(got), `\t`)
	require.True(t, bytes.Contains(got, []byte("\"msg\": \"database query\"")))
}
