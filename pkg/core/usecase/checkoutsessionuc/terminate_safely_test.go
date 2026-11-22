package checkoutsessionuc

import (
	"testing"

	"github.com/stretchr/testify/require"
	testifymock "github.com/stretchr/testify/mock"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestTerminateCheckoutSafely_OpenWithoutContractCancels(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	open := h.session(withoutDecision())
	canceled := h.session(
		withoutDecision(),
		withStatus(model.CanceledCheckoutSessionStatus),
	)

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().
		MarkCanceledSafely(h.ctx, h.sessionID, model.TechnicalFailureCheckoutReason, h.now).
		Return(canceled, nil).Once()
	h.expectLifecycleEvent(canceled, checkoutlifecycle.EventTypeCanceled)

	out, err := h.uc.TerminateCheckoutSafely(h.ctx, TerminateCheckoutSafelyInput{
		SessionID:      h.sessionID,
		Reason:         model.TechnicalFailureCheckoutReason,
		TerminalStatus: model.CanceledCheckoutSessionStatus,
	})
	require.NoError(t, err)
	require.False(t, out.AlreadyTerminal)
	require.Equal(t, model.CanceledCheckoutSessionStatus, out.Session.Status)
}

func TestTerminateCheckoutSafely_SubmittedAttemptBlocks(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	session := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
	)

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

	_, err := h.uc.TerminateCheckoutSafely(h.ctx, TerminateCheckoutSafelyInput{
		SessionID:      h.sessionID,
		Reason:         model.CustomerCanceledCheckoutReason,
		TerminalStatus: model.CanceledCheckoutSessionStatus,
	})
	requireCerr(
		t,
		err,
		cerr.CodeConflict,
		"error.checkout_payment_is_in_progress_and_must_be_resolved_first",
	)
}

func TestTerminateCheckoutSafely_IdempotentWhenAlreadyCanceled(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	canceled := h.session(withStatus(model.CanceledCheckoutSessionStatus))
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(canceled, nil).Once()

	out, err := h.uc.TerminateCheckoutSafely(h.ctx, TerminateCheckoutSafelyInput{
		SessionID:      h.sessionID,
		Reason:         model.CustomerCanceledCheckoutReason,
		TerminalStatus: model.CanceledCheckoutSessionStatus,
	})
	require.NoError(t, err)
	require.True(t, out.AlreadyTerminal)
}

func TestTerminateCheckoutSafely_ConfirmedMerchantEntitlementBlocks(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	session := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
	)
	entitlementID := "me_123"

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(session, nil).Once()
	h.contractTx.EXPECT().LockByID(h.ctx, h.contractID).Return(model.Contract{
		ID:                             h.contractID,
		Status:                         model.PaymentPendingContractStatus,
		FinancialMerchantEntitlementID: &entitlementID,
	}, nil).Once()

	_, err := h.uc.TerminateCheckoutSafely(h.ctx, TerminateCheckoutSafelyInput{
		SessionID:      h.sessionID,
		Reason:         model.TechnicalFailureCheckoutReason,
		TerminalStatus: model.CanceledCheckoutSessionStatus,
	})
	requireCerr(
		t,
		err,
		cerr.CodeConflict,
		"error.checkout_payment_is_in_progress_and_must_be_resolved_first",
	)

	_ = testifymock.Anything
}
