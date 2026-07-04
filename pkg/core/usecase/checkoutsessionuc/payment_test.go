package checkoutsessionuc

import (
	"encoding/json"
	"errors"
	"testing"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const paymentStateConflictEventType = "bnpl.checkout.payment_state_conflict.v1"

func notFoundSession() error {
	return cerr.ResourceNotFound(errors.New("no checkout session for contract"))
}

func TestApplyPaymentSucceeded_AdvancesInFlightSessionsToCompleted(t *testing.T) {
	t.Parallel()

	for _, status := range []model.CheckoutSessionStatus{
		model.ProcessingCheckoutSessionStatus,
		model.ConfirmingCheckoutSessionStatus,
	} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			inFlight := h.session(withStatus(status), withContract(h.contractID))
			completed := h.session(withStatus(model.CompletedCheckoutSessionStatus), withContract(h.contractID))

			h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(inFlight, nil).Once()
			h.sessionTx.EXPECT().
				UpdateStatus(h.ctx, h.sessionID, model.CompletedCheckoutSessionStatus, h.now).
				Return(completed, nil).Once()
			h.expectLifecycleEvent(completed, checkoutlifecycle.EventTypeCompleted)

			require.NoError(t, h.uc.ApplyPaymentSucceeded(h.ctx, h.tx, h.contractID, h.now))
		})
	}
}

func TestApplyPaymentSucceeded_DuplicateSuccessIsIdempotent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	processing := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	completed := h.session(withStatus(model.CompletedCheckoutSessionStatus), withContract(h.contractID))

	// First delivery completes the session and emits exactly one event.
	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(processing, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.CompletedCheckoutSessionStatus, h.now).
		Return(completed, nil).Once()
	h.expectLifecycleEvent(completed, checkoutlifecycle.EventTypeCompleted)

	// Redelivery sees completed: no UpdateStatus, no outbox row (strict mocks).
	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(completed, nil).Once()

	require.NoError(t, h.uc.ApplyPaymentSucceeded(h.ctx, h.tx, h.contractID, h.now))
	require.NoError(t, h.uc.ApplyPaymentSucceeded(h.ctx, h.tx, h.contractID, h.now))
}

func TestApplyPaymentSucceeded_UnknownContractIsIgnored(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(model.CheckoutSession{}, notFoundSession()).Once()

	require.NoError(t, h.uc.ApplyPaymentSucceeded(h.ctx, h.tx, h.contractID, h.now))
}

func TestApplyPaymentSucceeded_LateSuccessOnTerminalSessionRecordsConflict(t *testing.T) {
	t.Parallel()

	for _, status := range []model.CheckoutSessionStatus{
		model.CanceledCheckoutSessionStatus,
		model.ExpiredCheckoutSessionStatus,
		model.UnavailableCheckoutSessionStatus,
	} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			terminal := h.session(withStatus(status), withContract(h.contractID))

			h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(terminal, nil).Once()

			// Evidence is written, but the terminal status is never rewritten
			// (UpdateStatus is not expected).
			h.outboxTx.EXPECT().
				Create(h.ctx, testifymock.MatchedBy(func(message repo.CreateOutboxMessage) bool {
					if message.Destination != testMerchantEventsDestination ||
						message.EventType != paymentStateConflictEventType ||
						message.MessageKey != h.sessionID.String() {
						return false
					}

					var payload map[string]string
					if err := json.Unmarshal(message.Payload, &payload); err != nil {
						return false
					}

					return payload["code"] == "CHECKOUT_PAYMENT_STATE_CONFLICT" &&
						payload["checkout_session_id"] == h.sessionID.String() &&
						payload["contract_id"] == h.contractID.String() &&
						payload["status"] == string(status)
				})).
				Return(model.OutboxMessage{}, nil).Once()

			// Conflict evidence must commit in the same transaction as FC success.
			require.NoError(t, h.uc.ApplyPaymentSucceeded(h.ctx, h.tx, h.contractID, h.now))
		})
	}
}

func TestApplyPaymentSucceeded_ConflictEvidenceWriteFailureIsReturned(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	canceled := h.session(withStatus(model.CanceledCheckoutSessionStatus), withContract(h.contractID))
	cause := errors.New("outbox unavailable")

	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(canceled, nil).Once()
	h.outboxTx.EXPECT().Create(h.ctx, testifymock.Anything).Return(model.OutboxMessage{}, cause).Once()

	err := h.uc.ApplyPaymentSucceeded(h.ctx, h.tx, h.contractID, h.now)
	require.ErrorIs(t, err, cause)
}

func TestApplyPaymentRetryableFailure_ReturnsProcessingSessionToOpenKeepingContract(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	processing := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	reopened := h.session(withContract(h.contractID))

	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(processing, nil).Once()

	// Only the status moves. The Contract link and Decision stay in place, so
	// Confirm can mint a new payment attempt against the same Contract; no
	// LinkContract / contract / lifecycle writes are expected (strict mocks).
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.OpenCheckoutSessionStatus, h.now).
		Return(reopened, nil).Once()

	require.NoError(t, h.uc.ApplyPaymentRetryableFailure(h.ctx, h.tx, h.contractID, h.now))
	require.NotNil(t, reopened.ContractID)
	require.Equal(t, h.contractID, *reopened.ContractID)
	require.NotNil(t, reopened.DecisionID)
}

func TestApplyPaymentRetryableFailure_IgnoresStaleFailureOutsideProcessing(t *testing.T) {
	t.Parallel()

	statuses := []model.CheckoutSessionStatus{
		// Authoritative success already won; a stale FAILED must not regress it.
		model.CompletedCheckoutSessionStatus,
		model.OpenCheckoutSessionStatus,
		model.ConfirmingCheckoutSessionStatus,
		model.CanceledCheckoutSessionStatus,
		model.ExpiredCheckoutSessionStatus,
		model.UnavailableCheckoutSessionStatus,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			session := h.session(withStatus(status), withContract(h.contractID))
			h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(session, nil).Once()

			require.NoError(t, h.uc.ApplyPaymentRetryableFailure(h.ctx, h.tx, h.contractID, h.now))
		})
	}
}

func TestApplyPaymentRetryableFailure_StaleFailureAfterSuccessDoesNotRegressCompleted(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	processing := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	completed := h.session(withStatus(model.CompletedCheckoutSessionStatus), withContract(h.contractID))

	// SUCCEEDED lands first.
	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(processing, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.CompletedCheckoutSessionStatus, h.now).
		Return(completed, nil).Once()
	h.expectLifecycleEvent(completed, checkoutlifecycle.EventTypeCompleted)

	// A stale FAILED arrives afterwards and must be a no-op.
	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(completed, nil).Once()

	require.NoError(t, h.uc.ApplyPaymentSucceeded(h.ctx, h.tx, h.contractID, h.now))
	require.NoError(t, h.uc.ApplyPaymentRetryableFailure(h.ctx, h.tx, h.contractID, h.now))
}

func TestApplyPaymentRetryableFailure_UnknownContractIsIgnored(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(model.CheckoutSession{}, notFoundSession()).Once()

	require.NoError(t, h.uc.ApplyPaymentRetryableFailure(h.ctx, h.tx, h.contractID, h.now))
}

func TestApplyPaymentRetryableFailure_PropagatesLockFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	cause := errors.New("lock timeout")
	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(model.CheckoutSession{}, cause).Once()

	err := h.uc.ApplyPaymentRetryableFailure(h.ctx, h.tx, h.contractID, h.now)
	require.ErrorIs(t, err, cause)
}
