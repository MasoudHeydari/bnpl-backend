package checkoutsessionuc

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestExpireDueOpenSessions_ExpiresOpenSessionsAndEmitsOneEventEach(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	first := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		withExpiresAt(h.now.Add(-time.Minute)),
	)
	second := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		withExpiresAt(h.now.Add(-time.Hour)),
		func(s *model.CheckoutSession) { s.ID = model.CheckoutSessionID(uuid.New()) },
	)

	// The batch size configured on the use case bounds the sweep.
	h.sessionTx.EXPECT().
		ExpireOpenDue(h.ctx, h.now, h.uc.expiryBatchSize).
		Return([]model.CheckoutSession{first, second}, nil).Once()
	h.expectLifecycleEvent(first, checkoutlifecycle.EventTypeExpired)
	h.expectLifecycleEvent(second, checkoutlifecycle.EventTypeExpired)

	output, err := h.uc.ExpireDueOpenSessions(h.ctx)
	require.NoError(t, err)
	require.Equal(t, 2, output.ExpiredCount)
}

func TestExpireDueOpenSessions_LeavesConfirmingAndProcessingUntouched(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	// The repository only returns open sessions, so confirming and processing
	// rows past expires_at produce no result. No UpdateStatus and no outbox row
	// is expected (strict mocks).
	h.sessionTx.EXPECT().
		ExpireOpenDue(h.ctx, h.now, h.uc.expiryBatchSize).
		Return([]model.CheckoutSession{}, nil).Once()

	output, err := h.uc.ExpireDueOpenSessions(h.ctx)
	require.NoError(t, err)
	require.Zero(t, output.ExpiredCount)
}

func TestExpireDueOpenSessions_PropagatesRepositoryFailure(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	cause := errors.New("db down")
	h.sessionTx.EXPECT().
		ExpireOpenDue(h.ctx, h.now, h.uc.expiryBatchSize).
		Return(nil, cause).Once()

	_, err := h.uc.ExpireDueOpenSessions(h.ctx)
	require.ErrorIs(t, err, cause)
}

func TestTransitionToExpired(t *testing.T) {
	t.Parallel()

	t.Run("open session is expired and emits one expired event", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		open := h.session(withExpiresAt(h.now.Add(-time.Minute)))
		expired := h.session(
			withStatus(model.ExpiredCheckoutSessionStatus),
			withExpiresAt(h.now.Add(-time.Minute)),
		)

		h.sessionTx.EXPECT().
			UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, h.now).
			Return(expired, nil).Once()
		h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

		got, err := h.uc.transitionToExpired(h.ctx, h.tx, open, h.now)
		require.NoError(t, err)
		require.Equal(t, model.ExpiredCheckoutSessionStatus, got.Status)
	})

	t.Run("already expired session is idempotent and emits no event", func(t *testing.T) {
		t.Parallel()

		h := newHarness(t)

		expired := h.session(withStatus(model.ExpiredCheckoutSessionStatus))

		got, err := h.uc.transitionToExpired(h.ctx, h.tx, expired, h.now)
		require.NoError(t, err)
		require.Equal(t, expired, got)
	})

	blocked := []struct {
		name    string
		status  model.CheckoutSessionStatus
		message cerr.MessageKey
	}{
		{
			name:    "confirming is not expired",
			status:  model.ConfirmingCheckoutSessionStatus,
			message: "error.checkout_payment_is_in_progress_and_must_be_resolved_first",
		},
		{
			name:    "processing is not expired",
			status:  model.ProcessingCheckoutSessionStatus,
			message: "error.checkout_payment_is_in_progress_and_must_be_resolved_first",
		},
		{
			name:    "completed is not expired",
			status:  model.CompletedCheckoutSessionStatus,
			message: "error.checkout_session_cannot_continue_in_its_current_state",
		},
		{
			name:    "canceled is not expired",
			status:  model.CanceledCheckoutSessionStatus,
			message: "error.checkout_session_cannot_continue_in_its_current_state",
		},
		{
			name:    "unavailable is not expired",
			status:  model.UnavailableCheckoutSessionStatus,
			message: "error.checkout_session_cannot_continue_in_its_current_state",
		},
	}

	for _, tt := range blocked {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)

			// No UpdateStatus and no outbox write: strict mocks prove it.
			session := h.session(withStatus(tt.status), withExpiresAt(h.now.Add(-time.Hour)))

			_, err := h.uc.transitionToExpired(h.ctx, h.tx, session, h.now)
			requireCerr(t, err, cerr.CodeConflict, tt.message)
		})
	}
}

func TestConfirm_LazyExpiryPersistsExpiredAndEmitsLifecycleEvent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	pastDue := h.session(withExpiresAt(h.now.Add(-time.Second)))
	expired := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		withExpiresAt(h.now.Add(-time.Second)),
	)

	// The customer lock and MarkConfirming are never reached for an elapsed TTL.
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(pastDue, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, h.now).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	_, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
	require.Empty(t, h.contracts.calls)
}

func TestStart_LazyExpiryPersistsExpiredAndEmitsLifecycleEvent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	pastDue := h.session(withExpiresAt(h.now.Add(-time.Second)))
	expired := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		withExpiresAt(h.now.Add(-time.Second)),
	)

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(pastDue, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, h.now).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	_, err := h.uc.Start(h.ctx, StartInput{
		CheckoutSessionID: h.sessionID,
		IdentitySubjectID: h.profileID,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
	require.Empty(t, h.decisions.calls)
}

func TestConfirm_ConfirmingSessionPastTTLIsNotExpiredLazily(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// beginConfirm TX, load contract Conn, policy Conn, load attempt TX, finishConfirm TX.
	h.expectPool(2, 3)

	// Only open sessions expire lazily; confirming past TTL resumes recovery.
	confirming := h.session(
		withStatus(model.ConfirmingCheckoutSessionStatus),
		withContract(h.contractID),
		withExpiresAt(h.now.Add(-time.Hour)),
	)
	processing := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
		withExpiresAt(h.now.Add(-time.Hour)),
	)
	prepared := h.createdContract(true)
	prepared.Contract.DecisionID = h.decisionID

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.expectCustomerLock()

	h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(prepared.Contract, nil).Once()
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		PolicyID: "checkout_contract_approval", PolicyVersion: "v1",
	}, nil).Once()
	h.expectCheckoutPaymentAttemptLoad(true)

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.sessionTx.EXPECT().UpdateStatus(h.ctx, h.sessionID, model.ProcessingCheckoutSessionStatus, h.now).
		Return(processing, nil).Once()

	output, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.NoError(t, err)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, output.Session.Status)
	require.True(t, output.PaymentRequired)
}
