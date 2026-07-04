package returndestination_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
)

func TestValidateURL_Development(t *testing.T) {
	t.Parallel()

	policy := returndestination.Development()

	require.NoError(t, policy.ValidateURL("https://merchant.example/return"))
	require.NoError(t, policy.ValidateURL("http://localhost:3000/return"))
	require.NoError(t, policy.ValidateURL("http://127.0.0.1:3000/return"))
	require.NoError(t, policy.ValidateURL("http://[::1]:3000/return"))

	for _, raw := range []string{
		"http://merchant.example/return",
		"http://192.168.1.10/return",
		"http://10.0.0.5/return",
		"http://example.com/return",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, policy.ValidateURL(raw), returndestination.ErrInsecureScheme)
		})
	}
}

func TestValidateURL_StrictRequiresHTTPS(t *testing.T) {
	t.Parallel()

	policy := returndestination.Strict()

	require.NoError(t, policy.ValidateURL("https://merchant.example/return"))
	require.ErrorIs(t,
		policy.ValidateURL("http://localhost:3000/return"),
		returndestination.ErrInsecureScheme,
	)
	require.ErrorIs(t,
		policy.ValidateURL("http://merchant.example/return"),
		returndestination.ErrInsecureScheme,
	)
}

func TestValidateURL_RejectsWhitespaceAndControlChars(t *testing.T) {
	t.Parallel()

	policy := returndestination.Development()

	for _, raw := range []string{
		" https://merchant.example/return",
		"https://merchant.example/return ",
		"https://merchant.example/return\n",
		"https://merchant.example/return\t",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, policy.ValidateURL(raw), returndestination.ErrReturnURLInvalid)
		})
	}
}

func TestValidateURL_Invalid(t *testing.T) {
	t.Parallel()

	policy := returndestination.Development()

	tests := map[string]error{
		"":                               returndestination.ErrReturnURLEmpty,
		"javascript:alert(1)":            returndestination.ErrReturnURLInvalid,
		"relative/path":                  returndestination.ErrReturnURLInvalid,
		"//example.com/callback":         returndestination.ErrReturnURLInvalid,
		"example.com/callback":           returndestination.ErrReturnURLInvalid,
		"https://u:p@example.com/return": returndestination.ErrReturnURLInvalid,
		"https://example.com/r?a=1":      returndestination.ErrReturnURLInvalid,
		"https://example.com/r?":         returndestination.ErrReturnURLInvalid,
		"https://example.com/r#foo":      returndestination.ErrReturnURLInvalid,
		"https://example.com/r#":         returndestination.ErrReturnURLInvalid,
		"https://example.com/" + strings.Repeat("a", returndestination.MaxReturnURLLength): returndestination.ErrReturnURLTooLong,
	}

	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			require.ErrorIs(t, policy.ValidateURL(raw), want)
		})
	}
}
