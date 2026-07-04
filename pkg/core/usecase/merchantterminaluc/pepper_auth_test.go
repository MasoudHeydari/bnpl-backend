package merchantterminaluc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	fingerprinthmac "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/hmac"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
)

func TestAuthenticateMissingPepperFailsClosedPublicly(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	issued := h.provision()

	v2Only, err := fingerprinthmac.NewKeyRing("v2", map[string][]byte{
		"v2": []byte("pepper-v2-0123456789abcdef01234567"),
	})
	require.NoError(t, err)
	h.uc.hasher = v2Only

	_, err = h.uc.Authenticate(h.ctx, issued.TerminalID, issued.APIKey)
	requireCode(t, err, cerr.CodeUnauthorized)
	require.ErrorIs(t, err, fingerprint.ErrPepperKeyMissing)

	ce, ok := errors.AsType[*cerr.Error](err)
	require.True(t, ok)
	require.Equal(t, cerr.MessageKey("error.invalid_terminal_credentials"), ce.MessageKey)
	require.NotContains(t, err.Error(), issued.APIKey)
	require.Contains(t, err.Error(), "credential_id=")
	require.Contains(t, err.Error(), "terminal_id=")
}

func TestAuthenticateStillAcceptsMatchingPepperVersion(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	issued := h.provision()

	// Credential was hashed with harness default v1; keep v1 in the ring while
	// making v2 active for new hashes.
	ring, err := fingerprinthmac.NewKeyRing("v2", map[string][]byte{
		"v1": []byte("0123456789abcdef0123456789abcdef"),
		"v2": []byte("pepper-v2-0123456789abcdef01234567"),
	})
	require.NoError(t, err)
	h.uc.hasher = ring

	principal, err := h.uc.Authenticate(h.ctx, issued.TerminalID, issued.APIKey)
	require.NoError(t, err)
	require.Equal(t, issued.CredentialID, principal.CredentialID)
	require.Equal(t, issued.TerminalID, principal.TerminalID)
}
