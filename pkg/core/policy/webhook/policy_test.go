package webhook_test

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"

	webhookpolicy "github.com/parstis/bnpl-backend/pkg/core/policy/webhook"
)

func TestPolicyValidateURL_Strict(t *testing.T) {
	t.Parallel()

	policy := webhookpolicy.Strict()

	valid := []string{
		"https://hooks.example.com/bnpl",
		"https://hooks.example.com:8443/bnpl?x=1",
		// Non-empty query whose value ends with '?' must remain valid; HasSuffix("?")
		// would incorrectly reject this.
		"https://hooks.example.com/bnpl?token=abc?",
		"https://8.8.8.8/hook",
	}
	for _, raw := range valid {
		_, err := policy.ValidateURL(raw)
		require.NoError(t, err, raw)
	}

	invalid := map[string]error{
		"http://hooks.example.com/bnpl":          webhookpolicy.ErrInsecureScheme,
		"ftp://hooks.example.com/":               webhookpolicy.ErrInvalidURL,
		"/relative/path":                         webhookpolicy.ErrInvalidURL,
		"https:///nohost":                        webhookpolicy.ErrInvalidURL,
		"https://user:pass@hooks.example.com/":   webhookpolicy.ErrInvalidURL,
		"https://hooks.example.com/#frag":        webhookpolicy.ErrInvalidURL,
		"https://hooks.example.com/#":            webhookpolicy.ErrInvalidURL,
		"https://hooks.example.com?":             webhookpolicy.ErrInvalidURL,
		"https://hooks.example.com/?":            webhookpolicy.ErrInvalidURL,
		" https://hooks.example.com/":            webhookpolicy.ErrInvalidURL,
		"https://hooks.example.com:99999/":       webhookpolicy.ErrInvalidURL,
		"https://localhost/hook":                 webhookpolicy.ErrBlockedHost,
		"https://api.localhost/hook":             webhookpolicy.ErrBlockedHost,
		"https://127.0.0.1/hook":                 webhookpolicy.ErrBlockedAddress,
		"https://127.5.5.5/hook":                 webhookpolicy.ErrBlockedAddress,
		"https://[::1]/hook":                     webhookpolicy.ErrBlockedAddress,
		"https://10.0.0.5/hook":                  webhookpolicy.ErrBlockedAddress,
		"https://172.16.3.4/hook":                webhookpolicy.ErrBlockedAddress,
		"https://192.168.1.1/hook":               webhookpolicy.ErrBlockedAddress,
		"https://169.254.169.254/latest":         webhookpolicy.ErrBlockedAddress,
		"https://[fd00:ec2::254]/latest":         webhookpolicy.ErrBlockedAddress,
		"https://[fe80::1]/hook":                 webhookpolicy.ErrBlockedAddress,
		"https://[::ffff:127.0.0.1]/hook":        webhookpolicy.ErrBlockedAddress,
		"https://100.100.100.200/meta":           webhookpolicy.ErrBlockedAddress,
		"https://0.0.0.0/hook":                   webhookpolicy.ErrBlockedAddress,
		"https://metadata.google.internal/x":     webhookpolicy.ErrBlockedHost,
		"https://service.internal/x":             webhookpolicy.ErrBlockedHost,
		"https://printer.local/x":                webhookpolicy.ErrBlockedHost,
		"https://redis/x":                        webhookpolicy.ErrBlockedHost,
		"https://2130706433/x":                   webhookpolicy.ErrBlockedHost,
		"https://0x7f.1/x":                       webhookpolicy.ErrBlockedHost,
		"https://[64:ff9b::7f00:1]/x":            webhookpolicy.ErrBlockedAddress,
		"https://[2002:7f00:1::]/x":              webhookpolicy.ErrBlockedAddress,
		"https://224.0.0.1/x":                    webhookpolicy.ErrBlockedAddress,
		"https://hooks.example.com./":            nil,
		"mailto:ops@example.com":                 webhookpolicy.ErrInvalidURL,
		"https:hooks.example.com":                webhookpolicy.ErrInvalidURL,
		"https://[fe80::1%25eth0]/hook":          webhookpolicy.ErrBlockedAddress,
		"https://metadata/computeMetadata/v1/":   webhookpolicy.ErrBlockedHost,
		"https://kubernetes.default.svc/healthz": webhookpolicy.ErrBlockedHost,
	}
	for raw, want := range invalid {
		_, err := policy.ValidateURL(raw)
		if want == nil {
			require.NoError(t, err, raw)

			continue
		}

		require.ErrorIs(t, err, want, raw)
	}
}

func TestPolicyValidateURL_DevelopmentAllowsLoopbackHTTP(t *testing.T) {
	t.Parallel()

	policy := webhookpolicy.Development()

	for _, raw := range []string{
		"http://localhost:8080/hook",
		"http://127.0.0.1:9000/hook",
		"http://[::1]:9000/hook",
		"https://hooks.example.com/hook",
	} {
		_, err := policy.ValidateURL(raw)
		require.NoError(t, err, raw)
	}

	// Loopback is the only relaxation: private and metadata ranges stay blocked.
	for _, raw := range []string{
		"http://10.0.0.5/hook",
		"http://169.254.169.254/latest",
		"http://0.0.0.0/hook",
	} {
		_, err := policy.ValidateURL(raw)
		require.ErrorIs(t, err, webhookpolicy.ErrBlockedAddress, raw)
	}
}

func TestStrictBlocksLoopback(t *testing.T) {
	t.Parallel()

	policy := webhookpolicy.Strict()

	require.True(t, policy.RequireHTTPS)
	require.True(t, policy.IsBlockedAddr(netip.MustParseAddr("127.0.0.1")))
}

func TestPolicyValidateURL_EmptyQueryDelimiterUsesForceQuery(t *testing.T) {
	t.Parallel()

	policy := webhookpolicy.Strict()

	for _, raw := range []string{
		"https://hooks.example.com?",
		"https://hooks.example.com/?",
		"https://hooks.example.com/hook?",
	} {
		t.Run("reject/"+raw, func(t *testing.T) {
			t.Parallel()

			_, err := policy.ValidateURL(raw)
			require.ErrorIs(t, err, webhookpolicy.ErrInvalidURL)
		})
	}

	// Regression: a query that happens to end with '?' is still a non-empty query.
	_, err := policy.ValidateURL("https://hooks.example.com/hook?q=ends-with-?")
	require.NoError(t, err)
}
