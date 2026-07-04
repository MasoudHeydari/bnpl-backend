package merchantterminaluc

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func cerrAs(err error) (*cerr.Error, bool) {
	return errors.AsType[*cerr.Error](err)
}

func TestProvisionCreatesTerminalAndStoresOnlyHash(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	out := h.provision()

	require.False(t, out.Replayed)
	require.Equal(t, h.merchantID, out.MerchantID)
	require.False(t, out.TerminalID.IsZero())
	require.EqualValues(t, 1, out.CredentialVersion)
	require.NoError(t, merchantterminal.ValidateAPIKeyFormat(out.APIKey))
	require.Equal(t, h.now.Add(testDeliveryTTL), out.ExpiresAt)

	require.Len(t, h.terminals.credentials, 1)
	require.Equal(t, model.ActiveMerchantTerminalCredentialStatus, h.terminals.credentials[0].Status)
	require.NotEqual(t, out.APIKey, h.terminals.credentials[0].APIKeyHash)

	require.Len(t, h.terminals.deliveries, 1)
	require.NotContains(t, h.terminals.deliveries[0].APIKeyCiphertext, out.APIKey)
}

func TestProvisionReplaysSameKeyForSameIdempotencyKey(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()

	h.advance(time.Minute)

	again, err := h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID, IdempotencyKey: "provision-1"})
	require.NoError(t, err)

	require.True(t, again.Replayed)
	require.Equal(t, first.APIKey, again.APIKey)
	require.Equal(t, first.TerminalID, again.TerminalID)
	require.Len(t, h.terminals.terminals, 1)
}

func TestProvisionWithDifferentKeyIsConflict(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	_, err := h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID, IdempotencyKey: "another"})
	requireCode(t, err, cerr.CodeConflict)
}

func TestProvisionReplayUnavailableAfterExpiry(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	h.advance(testDeliveryTTL)

	_, err := h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID, IdempotencyKey: "provision-1"})
	requireCode(t, err, cerr.CodeIdempotencyConflict)
}

func TestProvisionRequiresActiveExistingMerchant(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.uc.Provision(h.ctx, ProvisionInput{
		MerchantID:     model.MerchantID(uuid.New()),
		IdempotencyKey: "k",
	})
	requireCode(t, err, cerr.CodeResourceNotFound)

	h.profiles.profiles[h.merchantID] = model.MerchantProfile{
		MerchantID: h.merchantID,
		Status:     model.SuspendedMerchantStatus,
	}

	_, err = h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID, IdempotencyKey: "k"})
	requireCode(t, err, cerr.CodeConflict)
	require.Empty(t, h.terminals.terminals)
}

func TestProvisionRetriesTerminalIDCollision(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.terminals.terminalIDTakenFailures = provisionMaxAttempts - 1

	out := h.provision()

	require.Equal(t, provisionMaxAttempts, h.terminals.createTerminalCalls)
	require.False(t, out.TerminalID.IsZero())
}

func TestProvisionGivesUpAfterRepeatedCollisions(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.terminals.terminalIDTakenFailures = provisionMaxAttempts

	_, err := h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID, IdempotencyKey: "k"})
	requireCode(t, err, cerr.CodeConflict)
	require.Equal(t, provisionMaxAttempts, h.terminals.createTerminalCalls)
	require.Empty(t, h.terminals.terminals)
}

func TestProvisionValidation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.uc.Provision(h.ctx, ProvisionInput{IdempotencyKey: "k"})
	requireCode(t, err, cerr.CodeInvalidArgument)

	_, err = h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID})
	requireCode(t, err, cerr.CodeInvalidArgument)

	long := make([]byte, maxIdempotencyKeyLength+1)
	for i := range long {
		long[i] = 'a'
	}

	_, err = h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID, IdempotencyKey: string(long)})
	requireCode(t, err, cerr.CodeInvalidArgument)
}

func TestNewRejectsMissingDependenciesAndBadConfig(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	valid := h.uc.cfg

	_, err := New(
		nil,
		h.profilesRepo,
		h.terminalsRepo,
		h.uc.hasher,
		h.uc.box,
		h.uc.clock,
		valid,
	)
	require.Error(t, err)

	_, err = New(
		h.pool,
		nil,
		h.terminalsRepo,
		h.uc.hasher,
		h.uc.box,
		h.uc.clock,
		valid,
	)
	require.Error(t, err)

	_, err = New(
		h.pool,
		h.profilesRepo,
		nil,
		h.uc.hasher,
		h.uc.box,
		h.uc.clock,
		valid,
	)
	require.Error(t, err)

	_, err = New(
		h.pool,
		h.profilesRepo,
		h.terminalsRepo,
		nil,
		h.uc.box,
		h.uc.clock,
		valid,
	)
	require.Error(t, err)

	_, err = New(
		h.pool,
		h.profilesRepo,
		h.terminalsRepo,
		h.uc.hasher,
		nil,
		h.uc.clock,
		valid,
	)
	require.Error(t, err)

	_, err = New(
		h.pool,
		h.profilesRepo,
		h.terminalsRepo,
		h.uc.hasher,
		h.uc.box,
		nil,
		valid,
	)
	require.Error(t, err)

	_, err = New(
		h.pool,
		h.profilesRepo,
		h.terminalsRepo,
		h.uc.hasher,
		h.uc.box,
		h.uc.clock,
		Config{
			DefaultGrace:        time.Hour,
			MaxGrace:            time.Minute,
			IssuanceDeliveryTTL: time.Minute,
		},
	)
	require.Error(t, err)

	_, err = New(
		h.pool,
		h.profilesRepo,
		h.terminalsRepo,
		h.uc.hasher,
		h.uc.box,
		h.uc.clock,
		Config{
			MaxGrace: time.Minute,
		},
	)
	require.Error(t, err)
}

func TestProvisionMarksDeliveryConsumedAndStoresFingerprint(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	require.Len(t, h.terminals.deliveries, 1)

	delivery := h.terminals.deliveries[0]
	require.Equal(t, provisionFingerprint(h.merchantID), delivery.RequestFingerprint)
	require.NotNil(t, delivery.ConsumedAt)
	require.Equal(t, h.now, *delivery.ConsumedAt)
}

func TestProvisionConsumedDeliveryStillReplays(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	first := h.provision()

	h.advance(time.Minute)

	again, err := h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID, IdempotencyKey: "provision-1"})
	require.NoError(t, err)
	require.True(t, again.Replayed)
	require.Equal(t, first.APIKey, again.APIKey)

	require.NotNil(t, h.terminals.deliveries[0].ConsumedAt)
	require.Equal(t, first.CreatedAt, *h.terminals.deliveries[0].ConsumedAt)
}

func TestProvisionFingerprintMismatchIsIdempotencyConflict(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	h.terminals.deliveries[0].RequestFingerprint = "v1|provision|someone-else"

	_, err := h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID, IdempotencyKey: "provision-1"})
	requireCode(t, err, cerr.CodeIdempotencyConflict)
}

func TestProvisionReplayRejectedAfterRotationSupersedesCredential(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	_, err := h.uc.Rotate(h.ctx, rotateInput(h, "rotate-1", nil))
	require.NoError(t, err)

	_, err = h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID, IdempotencyKey: "provision-1"})
	requireCode(t, err, cerr.CodeIdempotencyConflict)
}

func TestProvisionReplayRejectedAfterPurge(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	h.advance(testDeliveryTTL)

	scrubbed, err := h.uc.ScrubExpiredIssuanceDeliveries(h.ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, scrubbed)

	_, err = h.uc.Provision(h.ctx, ProvisionInput{MerchantID: h.merchantID, IdempotencyKey: "provision-1"})
	requireCode(t, err, cerr.CodeIdempotencyConflict)
	require.Len(t, h.terminals.terminals, 1)
}

func TestScrubExpiredIssuanceDeliveries(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.provision()

	scrubbed, err := h.uc.ScrubExpiredIssuanceDeliveries(h.ctx, 10)
	require.NoError(t, err)
	require.Zero(t, scrubbed, "unexpired deliveries are left alone")

	h.advance(testDeliveryTTL)

	scrubbed, err = h.uc.ScrubExpiredIssuanceDeliveries(h.ctx, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, scrubbed)

	delivery := h.terminals.deliveries[0]
	require.True(t, delivery.IsPurged())
	require.Empty(t, delivery.APIKeyCiphertext)
	require.Empty(t, delivery.APIKeyEncryptionKeyID)

	scrubbed, err = h.uc.ScrubExpiredIssuanceDeliveries(h.ctx, 10)
	require.NoError(t, err)
	require.Zero(t, scrubbed, "purged tombstones are not scrubbed twice")

	_, err = h.uc.ScrubExpiredIssuanceDeliveries(h.ctx, 0)
	require.Error(t, err)
}

func TestFingerprintsAreDeterministic(t *testing.T) {
	t.Parallel()

	id := model.MerchantID(uuid.MustParse("11111111-1111-1111-1111-111111111111"))

	require.Equal(t, "v1|provision|11111111-1111-1111-1111-111111111111", provisionFingerprint(id))
	require.Equal(
		t,
		"v1|rotate|11111111-1111-1111-1111-111111111111|3600000000000",
		rotateFingerprint(id, time.Hour),
	)
	require.NotEqual(t, rotateFingerprint(id, 0), rotateFingerprint(id, time.Nanosecond))
}
