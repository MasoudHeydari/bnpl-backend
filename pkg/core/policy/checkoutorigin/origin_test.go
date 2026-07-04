package checkoutorigin_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	checkoutorigin "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutorigin"
)

func TestParse_ProductionValid(t *testing.T) {
	t.Parallel()

	policy := checkoutorigin.Strict([]string{"checkout.bnpl.example.com"})

	for _, raw := range []string{
		"https://checkout.bnpl.example.com",
		"https://checkout.bnpl.example.com:443",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			origin, err := policy.Parse(raw)
			require.NoError(t, err)
			require.Equal(t, raw, origin.String())
		})
	}

	t.Run("uppercase scheme is canonicalized", func(t *testing.T) {
		t.Parallel()

		origin, err := policy.Parse("HTTPS://checkout.bnpl.example.com")
		require.NoError(t, err)
		require.Equal(t, "https://checkout.bnpl.example.com", origin.String())
	})
}

func TestParse_ProductionInvalid(t *testing.T) {
	t.Parallel()

	policy := checkoutorigin.Strict([]string{"checkout.bnpl.example.com"})

	tests := map[string]error{
		"":                                   checkoutorigin.ErrOriginEmpty,
		" ":                                  checkoutorigin.ErrOriginInvalid,
		"http://checkout.bnpl.example.com":   checkoutorigin.ErrInsecureScheme,
		"https://checkout.bnpl.example.com/": checkoutorigin.ErrOriginInvalid,
		"https://checkout.bnpl.example.com/checkout":         checkoutorigin.ErrOriginInvalid,
		"https://checkout.bnpl.example.com?":                 checkoutorigin.ErrOriginInvalid,
		"https://checkout.bnpl.example.com?foo=bar":          checkoutorigin.ErrOriginInvalid,
		"https://checkout.bnpl.example.com#":                 checkoutorigin.ErrOriginInvalid,
		"https://checkout.bnpl.example.com#foo":              checkoutorigin.ErrOriginInvalid,
		"https://checkout.bnpl.example.com.":                 checkoutorigin.ErrOriginInvalid,
		"https://user:pass@checkout.bnpl.example.com":        checkoutorigin.ErrOriginInvalid,
		"https://localhost":                                  checkoutorigin.ErrHostNotAllowed,
		"https://127.0.0.1":                                  checkoutorigin.ErrHostNotAllowed,
		"https://[::1]":                                      checkoutorigin.ErrHostNotAllowed,
		"https://10.0.0.1":                                   checkoutorigin.ErrHostNotAllowed,
		"https://192.168.1.1":                                checkoutorigin.ErrHostNotAllowed,
		"https://203.0.113.10":                               checkoutorigin.ErrHostNotAllowed,
		"https://wrong.example.com":                          checkoutorigin.ErrHostNotAllowed,
		" https://checkout.bnpl.example.com":                 checkoutorigin.ErrOriginInvalid,
		"https://checkout.bnpl.example.com ":                 checkoutorigin.ErrOriginInvalid,
		"https://checkout.bnpl.example.com:8443":             checkoutorigin.ErrPortNotAllowed,
		"https://evilcheckout.bnpl.example.com":              checkoutorigin.ErrHostNotAllowed,
		"https://checkout.bnpl.example.com.attacker.example": checkoutorigin.ErrHostNotAllowed,
	}

	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			_, err := policy.Parse(raw)
			require.ErrorIs(t, err, want)
		})
	}
}

func TestParse_StrictRequiresAllowedHosts(t *testing.T) {
	t.Parallel()

	_, err := checkoutorigin.Strict(nil).Parse("https://checkout.bnpl.example.com")
	require.ErrorIs(t, err, checkoutorigin.ErrAllowedHostsRequired)
}

func TestParse_TrailingDotNotEquivalentToAllowlist(t *testing.T) {
	t.Parallel()

	policy := checkoutorigin.Strict([]string{"checkout.bnpl.example.com"})

	_, err := policy.Parse("https://checkout.bnpl.example.com.")
	require.ErrorIs(t, err, checkoutorigin.ErrOriginInvalid)
}

func TestParse_DevelopmentValid(t *testing.T) {
	t.Parallel()

	policy := checkoutorigin.Development([]string{"bnpl.test", "localhost"})

	for _, raw := range []string{
		"https://bnpl.test",
		"http://localhost:3000",
		"http://127.0.0.1:3000",
		"http://[::1]:3000",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			origin, err := policy.Parse(raw)
			require.NoError(t, err)
			require.Equal(t, raw, origin.String())
		})
	}
}

func TestParse_DevelopmentInvalid(t *testing.T) {
	t.Parallel()

	policy := checkoutorigin.Development([]string{"bnpl.test", "localhost"})

	tests := map[string]error{
		"http://example.com":   checkoutorigin.ErrInsecureScheme,
		"http://192.168.1.10":  checkoutorigin.ErrInsecureScheme,
		"http://10.0.0.5":      checkoutorigin.ErrInsecureScheme,
		"https://example.com":  checkoutorigin.ErrHostNotAllowed,
		"https://192.168.1.10": checkoutorigin.ErrHostNotAllowed,
		"https://10.0.0.5":     checkoutorigin.ErrHostNotAllowed,
	}

	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			_, err := policy.Parse(raw)
			require.ErrorIs(t, err, want)
		})
	}
}

func TestParse_HostAllowlistIsExact(t *testing.T) {
	t.Parallel()

	policy := checkoutorigin.Strict([]string{"bnpl.example.com"})

	_, err := policy.Parse("https://notbnpl.example.com")
	require.ErrorIs(t, err, checkoutorigin.ErrHostNotAllowed)

	_, err = policy.Parse("https://bnpl.example.com")
	require.NoError(t, err)
}
