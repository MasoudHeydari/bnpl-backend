package merchantterminaluc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func rotateInput(h *harness, key string, grace *time.Duration) RotateInput {
	return RotateInput{MerchantID: h.merchantID, GracePeriod: grace, IdempotencyKey: key}
}

func TestRotateCreatesNextVersionAndRetiresPrevious(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()

	out, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	require.False(t, out.Replayed)
	require.EqualValues(t, 2, out.CredentialVersion)
	require.NotEmpty(t, out.APIKey)
	require.NotEqual(t, first.APIKey, out.APIKey)
	require.Equal(t, first.TerminalID, out.TerminalID)
	require.NotNil(t, out.Retiring)
	require.Equal(t, first.CredentialID, out.Retiring.CredentialID)
	require.EqualValues(t, 1, out.Retiring.Version)
	require.Equal(t, h.now.Add(testDefaultGrace), out.Retiring.RetireAt)
	require.Equal(t, h.now.Add(testDeliveryTTL), out.ExpiresAt)

	old, err := h.terminals.GetCredentialByID(h.ctx, first.CredentialID)
	require.NoError(t, err)
	require.Equal(t, model.RetiringMerchantTerminalCredentialStatus, old.Status)

	active, err := h.terminals.GetActiveCredential(h.ctx, old.MerchantTerminalID)
	require.NoError(t, err)
	require.Equal(t, out.CredentialID, active.ID)
	require.NotContains(t, active.APIKeyHash, out.APIKey)
}

func TestRotateZeroGraceRetiresPreviousImmediately(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()

	zero := time.Duration(0)

	out, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &zero))
	require.NoError(t, err)
	require.Nil(t, out.Retiring)

	old, err := h.terminals.GetCredentialByID(h.ctx, first.CredentialID)
	require.NoError(t, err)
	require.Equal(t, model.RetiredMerchantTerminalCredentialStatus, old.Status)
	require.NotNil(t, old.RetiredAt)
	require.Equal(t, h.now, *old.RetiredAt)
	require.Nil(t, old.RetireAt)

	_, err = h.uc.Authenticate(h.ctx, first.TerminalID, first.APIKey)
	requireCode(t, err, cerr.CodeUnauthorized)

	_, err = h.uc.Authenticate(h.ctx, first.TerminalID, out.APIKey)
	require.NoError(t, err)
}

func TestRotateRefusedWhileGraceIsRunning(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "rotate-2", nil))
	requireCode(t, err, cerr.CodeConflict)

	ce, ok := cerrAs(err)
	require.True(t, ok)
	require.Equal(t, "CREDENTIAL_ROTATION_IN_PROGRESS", ce.Details.Violations[0].Code)
}

func TestRotateAllowedAfterGraceElapsedAndRetiresStaleCredential(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	h.advance(testDefaultGrace)

	out, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-2", nil))
	require.NoError(t, err)
	require.EqualValues(t, 3, out.CredentialVersion)

	byVersion := map[int64]model.MerchantTerminalCredentialStatus{}
	for _, credential := range h.terminals.credentials {
		byVersion[credential.Version] = credential.Status
	}

	require.Equal(t, model.RetiredMerchantTerminalCredentialStatus, byVersion[1])
	require.Equal(t, model.RetiringMerchantTerminalCredentialStatus, byVersion[2])
	require.Equal(t, model.ActiveMerchantTerminalCredentialStatus, byVersion[3])
}

func TestRotateReplaysSameKeyForSameIdempotencyKey(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	first, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	credentials := len(h.terminals.credentials)

	h.advance(time.Minute)

	again, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	require.True(t, again.Replayed)
	require.Equal(t, first.APIKey, again.APIKey)
	require.Equal(t, first.CredentialID, again.CredentialID)
	require.NotNil(t, again.Retiring)
	require.Equal(t, first.Retiring.CredentialID, again.Retiring.CredentialID)
	require.Len(t, h.terminals.credentials, credentials)
}

func TestRotateReplayUnavailableAfterDeliveryExpiry(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	h.advance(testDeliveryTTL)

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	requireCode(t, err, cerr.CodeIdempotencyConflict)
}

func TestRotateDoesNotReplayProvisionKey(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	out, err := h.uc.Rotate(h.ctx, rotateInput(h, "provision-1", nil))
	require.NoError(t, err)
	require.False(t, out.Replayed)
	require.EqualValues(t, 2, out.CredentialVersion)
}

func TestRotateValidation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	negative := -time.Second
	tooLong := testMaxGrace + time.Second
	atMax := testMaxGrace

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "k", &negative))
	requireCode(t, err, cerr.CodeInvalidArgument)

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "k", &tooLong))
	requireCode(t, err, cerr.CodeInvalidArgument)

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "", nil))
	requireCode(t, err, cerr.CodeInvalidArgument)

	_, err = h.uc.Rotate(h.ctx, RotateInput{IdempotencyKey: "k"})
	requireCode(t, err, cerr.CodeInvalidArgument)

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "k", &atMax))
	require.NoError(t, err)
}

func TestRotateRequiresActiveTerminal(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()
	h.terminals.terminals[0].Status = model.RevokedMerchantTerminalStatus

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	requireCode(t, err, cerr.CodeConflict)
}

func TestRotateWithoutTerminalIsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	requireCode(t, err, cerr.CodeResourceNotFound)
}

func TestRotateReplayMatchesOmittedAndExplicitDefaultGrace(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	first, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	explicit := testDefaultGrace

	again, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &explicit))
	require.NoError(t, err)
	require.True(t, again.Replayed)
	require.Equal(t, first.APIKey, again.APIKey)
}

func TestRotateSameKeyDifferentGraceIsIdempotencyConflict(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	first, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	other := time.Hour

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &other))
	requireCode(t, err, cerr.CodeIdempotencyConflict)

	zero := time.Duration(0)

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &zero))
	requireCode(t, err, cerr.CodeIdempotencyConflict)

	// The conflicting attempts must not have rotated again.
	require.Len(t, h.terminals.credentials, 2)
	require.EqualValues(t, 2, first.CredentialVersion)
}

func TestRotateZeroGraceReplayHasNoRetiring(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	zero := time.Duration(0)

	first, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &zero))
	require.NoError(t, err)

	again, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &zero))
	require.NoError(t, err)
	require.True(t, again.Replayed)
	require.Equal(t, first.APIKey, again.APIKey)
	require.Nil(t, again.Retiring)
}

func TestRotateReplayRejectedAfterLaterRotation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	zero := time.Duration(0)

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &zero))
	require.NoError(t, err)

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "rotate-2", &zero))
	require.NoError(t, err)

	// rotate-1's credential is now retired; its key must not be replayed.
	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &zero))
	requireCode(t, err, cerr.CodeIdempotencyConflict)
}

func TestRotateReplayRejectedWhileCredentialIsRetiring(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	// Force the new credential into RETIRING as a later rotation would.
	active, err := h.terminals.GetActiveCredential(h.ctx, h.terminals.terminals[0].ID)
	require.NoError(t, err)

	_, err = h.terminals.MarkRetiring(h.ctx, active.ID, h.now.Add(time.Hour), h.now)
	require.NoError(t, err)

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	requireCode(t, err, cerr.CodeIdempotencyConflict)
}

func TestRotateConsumedDeliveryStillReplays(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	first, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	delivery, err := h.terminals.GetIssuanceDeliveryByIdempotencyKey(
		h.ctx,
		h.terminals.terminals[0].ID,
		rotateKeyPrefix+"rotate-1",
	)
	require.NoError(t, err)
	require.NotNil(t, delivery.ConsumedAt)

	consumedAt := *delivery.ConsumedAt

	h.advance(time.Minute)

	again, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)
	require.True(t, again.Replayed)
	require.Equal(t, first.APIKey, again.APIKey)

	delivery, err = h.terminals.GetIssuanceDeliveryByIdempotencyKey(
		h.ctx,
		h.terminals.terminals[0].ID,
		rotateKeyPrefix+"rotate-1",
	)
	require.NoError(t, err)
	require.Equal(t, consumedAt, *delivery.ConsumedAt, "first consumption time is kept")
}

func TestRotateReplayRejectedAfterPurge(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	h.advance(testDeliveryTTL)

	scrubbed, err := h.uc.ScrubExpiredIssuanceDeliveries(h.ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 2, scrubbed)

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	requireCode(t, err, cerr.CodeIdempotencyConflict)

	// A different grace on a tombstoned key is still a conflict, never a new key.
	other := time.Hour

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", &other))
	requireCode(t, err, cerr.CodeIdempotencyConflict)
	require.Len(t, h.terminals.credentials, 2)
}

func TestRotateReplayRejectedWhenPurgedBeforeExpiryCheck(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	// A purged row must be refused even if its expires_at looks in the future.
	for index := range h.terminals.deliveries {
		purgedAt := h.now
		h.terminals.deliveries[index].PurgedAt = &purgedAt
		h.terminals.deliveries[index].APIKeyCiphertext = ""
		h.terminals.deliveries[index].APIKeyEncryptionKeyID = ""
	}

	_, err = h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	requireCode(t, err, cerr.CodeIdempotencyConflict)
}
