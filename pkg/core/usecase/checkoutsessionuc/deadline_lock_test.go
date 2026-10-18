package checkoutsessionuc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

func TestMarkConfirmingOpenSession_ClockAdvancesPastDeadlineDuringLockWait(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	deadline := h.now.Add(time.Minute)
	afterDeadline := deadline.Add(time.Second)
	clk := newSettableClock(h.now)
	h.uc.clock = clk

	open := h.session(withExpiresAt(deadline))
	expired := h.session(withExpiresAt(deadline), withStatus(model.ExpiredCheckoutSessionStatus))

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).RunAndReturn(
		func(context.Context, model.CheckoutSessionID) (model.CheckoutSession, error) {
			// Simulate waiting past expires_at before the row lock is held.
			clk.set(afterDeadline)

			return open, nil
		},
	).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, afterDeadline).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	_, err := h.uc.markConfirmingOpenSession(h.ctx, open)
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
}

func TestBumpEvaluationGenerationCAS_ClockAdvancesPastDeadlineDuringLockWait(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	deadline := h.now.Add(time.Minute)
	afterDeadline := deadline.Add(time.Second)
	clk := newSettableClock(h.now)
	h.uc.clock = clk

	open := h.session(withExpiresAt(deadline))
	expired := h.session(withExpiresAt(deadline), withStatus(model.ExpiredCheckoutSessionStatus))

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).RunAndReturn(
		func(context.Context, model.CheckoutSessionID) (model.CheckoutSession, error) {
			clk.set(afterDeadline)

			return open, nil
		},
	).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, afterDeadline).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	_, err := h.uc.bumpEvaluationGenerationCAS(h.ctx, open)
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
}

func TestRecordEvaluation_ClockAdvancesPastDeadlineAfterDecisionCreate(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	deadline := h.now.Add(time.Minute)
	afterDeadline := deadline.Add(time.Second)
	clk := newSettableClock(h.now)
	h.uc.clock = clk

	open := h.session(withExpiresAt(deadline), withoutDecision(), withGeneration(2))
	expired := h.session(
		withExpiresAt(deadline),
		withoutDecision(),
		withGeneration(2),
		withStatus(model.ExpiredCheckoutSessionStatus),
	)

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).RunAndReturn(
		func(context.Context, model.CheckoutSessionID) (model.CheckoutSession, error) {
			clk.set(afterDeadline)

			return open, nil
		},
	).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, afterDeadline).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	_, err := h.uc.recordEvaluation(
		h.ctx,
		h.sessionID,
		2,
		h.decisionID,
		model.OpenCheckoutSessionStatus,
		"",
	)
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
}

func TestLockExternalContinuable_ReturnsPersistedExpiredRow(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	open := h.session(withExpiresAt(h.now.Add(-time.Second)))
	expired := h.session(
		withExpiresAt(h.now.Add(-time.Second)),
		withStatus(model.ExpiredCheckoutSessionStatus),
	)

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(open, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, h.now).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	got, wasExpired, err := h.uc.lockExternalContinuable(h.ctx, h.tx, h.publicToken)
	require.NoError(t, err)
	require.True(t, wasExpired)
	require.Equal(t, model.ExpiredCheckoutSessionStatus, got.Status)
	require.Equal(t, expired, got)
}

func TestPersistExternalUnavailable_HardDeadlineWinsOverLateIdentityMiss(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	deadline := h.now.Add(time.Minute)
	afterDeadline := deadline.Add(time.Second)
	clk := newSettableClock(h.now)
	h.uc.clock = clk

	open := h.session(
		withExpiresAt(deadline),
		withoutDecision(),
		withCustomer(nil),
	)
	expired := h.session(
		withExpiresAt(deadline),
		withoutDecision(),
		withCustomer(nil),
		withStatus(model.ExpiredCheckoutSessionStatus),
	)

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).RunAndReturn(
		func(context.Context, model.CheckoutSessionToken) (model.CheckoutSession, error) {
			clk.set(afterDeadline)

			return open, nil
		},
	).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, afterDeadline).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	got, err := h.uc.persistExternalUnavailable(h.ctx, h.publicToken)
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
	require.Equal(t, model.ExpiredCheckoutSessionStatus, got.Status)
}

func TestEvaluateSession_DurableUnavailableWinsOverLocalApprovedOffer(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// prepareEvaluationGeneration Conn + recordEvaluation Conn/Tx
	h.expectPool(1, 1)

	open := h.session()
	unavailable := h.session(withStatus(model.UnavailableCheckoutSessionStatus))

	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		ID:        h.decisionID,
		ExpiresAt: ptrTime(h.now.Add(15 * time.Minute)),
	}, nil).Once()

	approvedID := model.DecisionID(uuid.New())
	h.decisions.output = decisionuc.CreateOutput{
		Decision:                    model.Decision{ID: approvedID, Outcome: model.ApprovedDecisionOutcome},
		RequestedAmountMinor:        150000000,
		ApprovedFinancedAmountMinor: 150000000,
		Currency:                    model.IRRCurrency,
		ExpiresAt:                   ptrTime(h.now.Add(15 * time.Minute)),
	}

	// Concurrent denied winner already terminalized the session.
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(unavailable, nil).Once()

	got, offer, err := h.uc.evaluateSession(h.ctx, open, h.profileID)
	require.NoError(t, err)
	require.Equal(t, model.UnavailableCheckoutSessionStatus, got.Status)
	require.Equal(t, DeniedOfferOutcome, offer.Outcome)
	require.Len(t, h.decisions.calls, 1)
}

func TestReopenUnlinkedConfirming_ExpiresInSameTransactionAfterDeadline(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	deadline := h.now.Add(-time.Second)
	confirming := h.confirmingSession(withExpiresAt(deadline))
	reopened := h.session(withExpiresAt(deadline))
	expired := h.session(withExpiresAt(deadline), withStatus(model.ExpiredCheckoutSessionStatus))

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.contractTx.EXPECT().GetByDecisionID(h.ctx, h.decisionID).
		Return(model.Contract{}, cerr.ResourceNotFound(nil)).Once()
	h.sessionTx.EXPECT().
		ReopenConfirmingToOpen(h.ctx, h.sessionID, h.confirmationAttemptID, h.now).
		Return(reopened, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, h.now).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	_, err := h.uc.reopenUnlinkedConfirming(h.ctx, confirming)
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
}

func TestRefreshOfferForReconfirm_UnavailableSessionNeverReturnsApprovedOffer(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// No Decision yet → prepareEvaluationGeneration is a no-op Conn; only
	// recordEvaluation opens Conn/Tx after Create.
	h.expectPool(0, 1)

	open := h.session(withGeneration(2), withoutDecision())
	unavailable := h.session(
		withGeneration(2),
		withoutDecision(),
		withStatus(model.UnavailableCheckoutSessionStatus),
	)

	approvedID := model.DecisionID(uuid.New())
	h.decisions.output = decisionuc.CreateOutput{
		Decision:                    model.Decision{ID: approvedID, Outcome: model.ApprovedDecisionOutcome},
		RequestedAmountMinor:        150000000,
		ApprovedFinancedAmountMinor: 150000000,
		Currency:                    model.IRRCurrency,
		ExpiresAt:                   ptrTime(h.now.Add(15 * time.Minute)),
	}

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(unavailable, nil).Once()

	got, offer, err := h.uc.refreshOfferForReconfirm(h.ctx, open, h.profileID, false, true)
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_unavailable")
	require.Equal(t, model.UnavailableCheckoutSessionStatus, got.Status)
	require.Equal(t, DeniedOfferOutcome, offer.Outcome)
}

func TestClampExternalSessionToOffer_ExpiresWhenClampedDeadlinePassesDuringMutation(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	sessionDeadline := h.now.Add(10 * time.Minute)
	offerExpiry := h.now.Add(time.Minute)
	afterClamp := offerExpiry.Add(time.Second)
	clk := newSettableClock(h.now)
	h.uc.clock = clk

	open := h.session(withExpiresAt(sessionDeadline))
	clampedOpen := h.session(withExpiresAt(offerExpiry))
	expired := h.session(withExpiresAt(offerExpiry), withStatus(model.ExpiredCheckoutSessionStatus))

	h.sessionTx.EXPECT().
		ClampExpiresAt(h.ctx, h.sessionID, offerExpiry, h.now).
		RunAndReturn(func(
			_ context.Context,
			_ model.CheckoutSessionID,
			_ time.Time,
			_ time.Time,
		) (model.CheckoutSession, error) {
			// Simulate UPDATE waiting for the row lock past the newly-clamped deadline.
			clk.set(afterClamp)

			return clampedOpen, nil
		}).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, afterClamp).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	got, err := h.uc.clampExternalSessionToOffer(h.ctx, open, Offer{
		Outcome:        ApprovedOfferOutcome,
		OfferExpiresAt: &offerExpiry,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
	require.Equal(t, model.ExpiredCheckoutSessionStatus, got.Status)
	require.Equal(t, expired, got)
	require.Empty(t, h.decisions.calls)
	require.Empty(t, h.contracts.calls)
	require.Empty(t, h.notificationCalls)
}

func TestClampExternalSessionToOffer_KeepsOpenWhenClampedDeadlineStillFuture(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	sessionDeadline := h.now.Add(10 * time.Minute)
	offerExpiry := h.now.Add(time.Minute)
	open := h.session(withExpiresAt(sessionDeadline))
	clamped := h.session(withExpiresAt(offerExpiry))

	h.sessionTx.EXPECT().
		ClampExpiresAt(h.ctx, h.sessionID, offerExpiry, h.now).
		Return(clamped, nil).Once()

	got, err := h.uc.clampExternalSessionToOffer(h.ctx, open, Offer{
		Outcome:        ApprovedOfferOutcome,
		OfferExpiresAt: &offerExpiry,
	})
	require.NoError(t, err)
	require.Equal(t, model.OpenCheckoutSessionStatus, got.Status)
	require.Equal(t, offerExpiry, got.ExpiresAt)
}

func TestClampExternalSessionToOffer_NoOpWhenSessionDeadlineAlreadyEarlier(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	sessionDeadline := h.now.Add(2 * time.Minute)
	laterOfferExpiry := h.now.Add(15 * time.Minute)
	open := h.session(withExpiresAt(sessionDeadline))

	got, err := h.uc.clampExternalSessionToOffer(h.ctx, open, Offer{
		Outcome:        ApprovedOfferOutcome,
		OfferExpiresAt: &laterOfferExpiry,
	})
	require.NoError(t, err)
	require.Equal(t, open, got)
	require.Equal(t, sessionDeadline, got.ExpiresAt)
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
