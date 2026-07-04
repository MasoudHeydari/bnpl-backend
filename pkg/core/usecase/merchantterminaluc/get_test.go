package merchantterminaluc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestGetReturnsTerminalWithoutSecret(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	issued := h.provision()

	out, err := h.uc.Get(h.ctx, GetInput{MerchantID: h.merchantID})
	require.NoError(t, err)
	require.Len(t, out.Items, 1)

	item := out.Items[0]
	require.Equal(t, h.merchantID, item.MerchantID)
	require.Equal(t, issued.TerminalID, item.TerminalID)
	require.Equal(t, model.ActiveMerchantTerminalStatus, item.Status)
	require.NotNil(t, item.ActiveCredential)
	require.Equal(t, issued.CredentialID, item.ActiveCredential.CredentialID)
	require.Equal(t, issued.CredentialVersion, item.ActiveCredential.Version)
	require.Nil(t, item.Retiring)
}

func TestGetIncludesRetiringCredential(t *testing.T) {
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

	out, err := h.uc.Get(h.ctx, GetInput{MerchantID: h.merchantID})
	require.NoError(t, err)
	require.Len(t, out.Items, 1)

	item := out.Items[0]
	require.Equal(t, rotated.TerminalID, item.TerminalID)
	require.NotNil(t, item.ActiveCredential)
	require.Equal(t, rotated.CredentialID, item.ActiveCredential.CredentialID)
	require.NotNil(t, item.Retiring)
	require.Equal(t, first.CredentialID, item.Retiring.CredentialID)
}

func TestGetMissingTerminalReturnsEmptyList(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	out, err := h.uc.Get(h.ctx, GetInput{MerchantID: h.merchantID})
	require.NoError(t, err)
	require.NotNil(t, out.Items)
	require.Empty(t, out.Items)
}

func TestGetRequiresMerchantID(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.uc.Get(h.ctx, GetInput{})
	requireCode(t, err, cerr.CodeInvalidArgument)
}

func TestGetOmitsRetiringAfterZeroGraceRotation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	zero := time.Duration(0)

	rotated, err := h.uc.Rotate(h.ctx, RotateInput{
		MerchantID:     h.merchantID,
		GracePeriod:    &zero,
		IdempotencyKey: "rotate-1",
	})
	require.NoError(t, err)

	out, err := h.uc.Get(h.ctx, GetInput{MerchantID: h.merchantID})
	require.NoError(t, err)
	require.Len(t, out.Items, 1)
	require.NotNil(t, out.Items[0].ActiveCredential)
	require.Equal(t, rotated.CredentialID, out.Items[0].ActiveCredential.CredentialID)
	require.Nil(t, out.Items[0].Retiring)
}
