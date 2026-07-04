package merchantterminaluc

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestAuthenticateAcceptsActiveKey(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	issued := h.provision()

	principal, err := h.uc.Authenticate(h.ctx, issued.TerminalID, issued.APIKey)
	require.NoError(t, err)

	require.Equal(t, h.merchantID, principal.MerchantID)
	require.Equal(t, issued.TerminalID, principal.TerminalID)
	require.Equal(t, issued.CredentialID, principal.CredentialID)
	require.EqualValues(t, 1, principal.CredentialVersion)
	require.Equal(t, merchantterminal.InitialPartnerTerminalPermissions(), principal.Permissions)
}

func TestAuthenticateWritesNothing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	issued := h.provision()
	writes := h.terminals.writes

	_, err := h.uc.Authenticate(h.ctx, issued.TerminalID, issued.APIKey)
	require.NoError(t, err)

	_, err = h.uc.Authenticate(h.ctx, issued.TerminalID, wrongKey(t))
	require.Error(t, err)

	require.Equal(t, writes, h.terminals.writes)
}

func TestAuthenticateRejectionsShareOnePublicError(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	issued := h.provision()

	cases := map[string]func() error{
		"wrong key": func() error {
			_, err := h.uc.Authenticate(h.ctx, issued.TerminalID, wrongKey(t))

			return err
		},
		"malformed key": func() error {
			_, err := h.uc.Authenticate(h.ctx, issued.TerminalID, "not-a-key")

			return err
		},
		"empty key": func() error {
			_, err := h.uc.Authenticate(h.ctx, issued.TerminalID, "")

			return err
		},
		"unknown terminal": func() error {
			_, err := h.uc.Authenticate(h.ctx, issued.TerminalID+1, issued.APIKey)

			return err
		},
		"zero terminal": func() error {
			_, err := h.uc.Authenticate(h.ctx, 0, issued.APIKey)

			return err
		},
		"suspended terminal": func() error {
			h.terminals.terminals[0].Status = model.SuspendedMerchantTerminalStatus
			defer func() { h.terminals.terminals[0].Status = model.ActiveMerchantTerminalStatus }()

			_, err := h.uc.Authenticate(h.ctx, issued.TerminalID, issued.APIKey)

			return err
		},
	}

	for name, run := range cases {
		err := run()

		requireCode(t, err, cerr.CodeUnauthorized)

		ce, ok := errors.AsType[*cerr.Error](err)
		require.True(t, ok, name)
		require.Equal(t, cerr.MessageKey("error.invalid_terminal_credentials"), ce.MessageKey, name)
		require.NotContains(t, err.Error(), issued.APIKey, name)
	}
}

func TestAuthenticateAcceptsRetiringKeyUntilDeadline(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()

	grace := time.Hour
	rotated, err := h.uc.Rotate(h.ctx, RotateInput{
		MerchantID:     h.merchantID,
		GracePeriod:    &grace,
		IdempotencyKey: "rotate-1",
	})
	require.NoError(t, err)

	principal, err := h.uc.Authenticate(h.ctx, first.TerminalID, first.APIKey)
	require.NoError(t, err)
	require.EqualValues(t, 1, principal.CredentialVersion)

	principal, err = h.uc.Authenticate(h.ctx, first.TerminalID, rotated.APIKey)
	require.NoError(t, err)
	require.EqualValues(t, 2, principal.CredentialVersion)

	h.advance(time.Hour)

	_, err = h.uc.Authenticate(h.ctx, first.TerminalID, first.APIKey)
	requireCode(t, err, cerr.CodeUnauthorized)

	_, err = h.uc.Authenticate(h.ctx, first.TerminalID, rotated.APIKey)
	require.NoError(t, err)
}

func wrongKey(t *testing.T) string {
	t.Helper()

	key, err := merchantterminal.GenerateAPIKey(zeroReader{})
	require.NoError(t, err)

	return key
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)

	return len(p), nil
}
