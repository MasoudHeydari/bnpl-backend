package schemarp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRolePasswordLiteralEscaping(t *testing.T) {
	require.Equal(t, "E'quote''password'", passwordLiteral("quote'password"))
	require.Equal(t, `E'back\\slash'`, passwordLiteral(`back\slash`))
	require.Equal(t, "E'''; DROP SCHEMA bnpl CASCADE; --'", passwordLiteral("'; DROP SCHEMA bnpl CASCADE; --"))
}
