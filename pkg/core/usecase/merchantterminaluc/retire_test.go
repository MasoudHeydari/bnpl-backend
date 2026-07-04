package merchantterminaluc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestRetireMovesRetiringToRetiredAndRejectsOldKey(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	_, err = h.uc.Authenticate(h.ctx, first.TerminalID, first.APIKey)
	require.NoError(t, err)

	out, err := h.uc.Retire(h.ctx, RetireInput{MerchantID: h.merchantID, CredentialID: first.CredentialID})
	require.NoError(t, err)
	require.Equal(t, first.CredentialID, out.CredentialID)
	require.EqualValues(t, 1, out.Version)
	require.Equal(t, h.now, out.RetiredAt)

	_, err = h.uc.Authenticate(h.ctx, first.TerminalID, first.APIKey)
	requireCode(t, err, cerr.CodeUnauthorized)
}

func TestRetireIsIdempotentForRetiredCredential(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	input := RetireInput{MerchantID: h.merchantID, CredentialID: first.CredentialID}

	firstOut, err := h.uc.Retire(h.ctx, input)
	require.NoError(t, err)

	writes := h.terminals.writes

	h.advance(time.Hour)

	again, err := h.uc.Retire(h.ctx, input)
	require.NoError(t, err)
	require.Equal(t, firstOut, again)
	require.Equal(t, writes, h.terminals.writes)
}

func TestRetireRejectsActiveCredential(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()

	_, err := h.uc.Retire(h.ctx, RetireInput{MerchantID: h.merchantID, CredentialID: first.CredentialID})
	requireCode(t, err, cerr.CodeConflict)

	active, err := h.terminals.GetCredentialByID(h.ctx, first.CredentialID)
	require.NoError(t, err)
	require.Equal(t, model.ActiveMerchantTerminalCredentialStatus, active.Status)
}

func TestRetireRejectsCredentialOfAnotherTerminal(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	otherMerchant := model.MerchantID(uuid.New())
	h.profiles.profiles[otherMerchant] = model.MerchantProfile{
		MerchantID: otherMerchant,
		Status:     model.ActiveMerchantStatus,
	}

	other, err := h.uc.Provision(h.ctx, ProvisionInput{MerchantID: otherMerchant, IdempotencyKey: "other"})
	require.NoError(t, err)

	_, err = h.uc.Retire(h.ctx, RetireInput{MerchantID: h.merchantID, CredentialID: other.CredentialID})
	requireCode(t, err, cerr.CodeResourceNotFound)
}

func TestRetireValidationAndMissing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.uc.Retire(h.ctx, RetireInput{CredentialID: model.MerchantTerminalCredentialID(uuid.New())})
	requireCode(t, err, cerr.CodeInvalidArgument)

	_, err = h.uc.Retire(h.ctx, RetireInput{MerchantID: h.merchantID})
	requireCode(t, err, cerr.CodeInvalidArgument)

	_, err = h.uc.Retire(h.ctx, RetireInput{
		MerchantID:   h.merchantID,
		CredentialID: model.MerchantTerminalCredentialID(uuid.New()),
	})
	requireCode(t, err, cerr.CodeResourceNotFound)

	h.provision()

	_, err = h.uc.Retire(h.ctx, RetireInput{
		MerchantID:   h.merchantID,
		CredentialID: model.MerchantTerminalCredentialID(uuid.New()),
	})
	requireCode(t, err, cerr.CodeResourceNotFound)
}
