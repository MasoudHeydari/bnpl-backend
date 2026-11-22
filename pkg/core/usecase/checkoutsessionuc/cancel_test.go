package checkoutsessionuc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCancelByToken_ProcessingWithUnresolvedPaymentBlocks(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	session := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
	)

	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(session, nil).Once()
	// terminateCheckoutSafelyTx locks again inside the same transaction.
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(session, nil).Once()
	h.contractTx.EXPECT().LockByID(h.ctx, h.contractID).Return(model.Contract{
		ID:     h.contractID,
		Status: model.PaymentPendingContractStatus,
	}, nil).Once()

	attempt := model.PaymentAttempt{
		ID:    h.attemptID,
		State: model.SubmittedPaymentAttemptState,
	}
	h.attemptTx.EXPECT().
		GetByContractIDAndPurposeWithItems(
			h.ctx,
			h.contractID,
			model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		).
		Return(attempt, nil).Once()
	h.attemptTx.EXPECT().
		LockByIDWithItems(h.ctx, h.attemptID).
		Return(attempt, nil).Once()

	_, err := h.uc.CancelByToken(h.ctx, CancelByTokenInput{Token: h.publicToken})
	requireCerr(
		t,
		err,
		cerr.CodeConflict,
		"error.checkout_payment_is_in_progress_and_must_be_resolved_first",
	)
}

func TestCancelByToken_ConfirmingWithPreparedContractBlocks(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	session := h.confirmingSession()
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(session, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(session, nil).Once()
	h.contractTx.EXPECT().
		GetByDecisionID(h.ctx, h.decisionID).
		Return(model.Contract{ID: h.contractID, Status: model.PaymentPendingContractStatus}, nil).
		Once()

	_, err := h.uc.CancelByToken(h.ctx, CancelByTokenInput{Token: h.publicToken})
	requireCerr(
		t,
		err,
		cerr.CodeConflict,
		"error.checkout_payment_is_in_progress_and_must_be_resolved_first",
	)
}

func TestCancelByToken_AfterTTLExpiresSessionInsteadOfCanceling(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	pastDue := h.session(withExpiresAt(h.now.Add(-time.Minute)))
	expired := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		withExpiresAt(h.now.Add(-time.Minute)),
	)

	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(pastDue, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(pastDue, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, h.now).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	output, err := h.uc.CancelByToken(h.ctx, CancelByTokenInput{Token: h.publicToken})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
	require.Empty(t, output.ReturnURL)
	require.Equal(t, model.CheckoutSession{}, output.Session)
}

func TestCancelByToken_OpenSessionIsCanceledWithCanceledEvent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	open := h.session()
	canceled := h.session(withStatus(model.CanceledCheckoutSessionStatus))

	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(open, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().
		MarkCanceled(h.ctx, h.sessionID, model.CustomerCanceledCheckoutReason, h.now).
		Return(canceled, nil).Once()
	h.expectLifecycleEvent(canceled, checkoutlifecycle.EventTypeCanceled)

	output, err := h.uc.CancelByToken(h.ctx, CancelByTokenInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, model.CanceledCheckoutSessionStatus, output.Session.Status)
	require.Contains(t, output.ReturnURL, "checkout_session_id="+h.publicToken.String())
}

func TestCancelByToken_OpenWithSafeContractTerminates(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	session := h.session(withContract(h.contractID))
	canceled := h.session(
		withContract(h.contractID),
		withStatus(model.CanceledCheckoutSessionStatus),
	)

	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(session, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(session, nil).Once()

	contractRow := model.Contract{
		ID:         h.contractID,
		Status:     model.PaymentPendingContractStatus,
		DecisionID: h.decisionID,
	}
	h.contractTx.EXPECT().LockByID(h.ctx, h.contractID).Return(contractRow, nil).Once()
	h.attemptTx.EXPECT().
		GetByContractIDAndPurposeWithItems(
			h.ctx,
			h.contractID,
			model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		).
		Return(model.PaymentAttempt{}, cerr.ResourceNotFound(nil)).Once()

	// Initial-payment activation mode → orphan contract without attempt is safe.
	h.decisionTx.EXPECT().
		GetByID(h.ctx, h.decisionID).
		Return(model.Decision{
			ID:            h.decisionID,
			PolicyID:      "checkout_contract_approval",
			PolicyVersion: "v1",
		}, nil).Once()

	h.contractTx.EXPECT().LockByID(h.ctx, h.contractID).Return(contractRow, nil).Once()
	h.contractTx.EXPECT().
		MarkCanceledFromPaymentPending(h.ctx, h.contractID, h.now, h.now).
		Return(contractRow, nil).Once()
	h.attemptTx.EXPECT().
		GetByContractIDAndPurposeWithItems(
			h.ctx,
			h.contractID,
			model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		).
		Return(model.PaymentAttempt{}, cerr.ResourceNotFound(nil)).Once()
	h.installTx.EXPECT().
		ExpireOfferedByContractID(h.ctx, h.contractID, h.now).
		Return(int64(0), nil).Once()
	h.chargeTx.EXPECT().
		CancelOfferedByContractID(h.ctx, h.contractID, h.now).
		Return(int64(0), nil).Once()

	h.sessionTx.EXPECT().
		MarkCanceledSafely(h.ctx, h.sessionID, model.CustomerCanceledCheckoutReason, h.now).
		Return(canceled, nil).Once()
	h.expectLifecycleEvent(canceled, checkoutlifecycle.EventTypeCanceled)

	output, err := h.uc.CancelByToken(h.ctx, CancelByTokenInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, model.CanceledCheckoutSessionStatus, output.Session.Status)
}
