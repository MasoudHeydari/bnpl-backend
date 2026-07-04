package checkoutsessionuc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCancelByCustomer_RefusesSessionOwnedByConfirmOrPayment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status model.CheckoutSessionStatus
	}{
		{name: "confirming claim blocks cancel", status: model.ConfirmingCheckoutSessionStatus},
		{name: "processing payment blocks cancel", status: model.ProcessingCheckoutSessionStatus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.expectPool(0, 1)

			// MarkCanceled, UpdateStatus, and outbox writes are not expected:
			// any of them fails the strict mocks.
			h.sessionTx.EXPECT().
				LockByID(h.ctx, h.sessionID).
				Return(h.session(withStatus(tt.status)), nil).Once()
			h.expectCustomerLock()

			_, err := h.uc.CancelByCustomer(h.ctx, CancelByCustomerInput{
				CheckoutSessionID: h.sessionID,
				IdentitySubjectID: h.profileID,
			})
			requireCerr(
				t,
				err,
				cerr.CodeConflict,
				"error.checkout_payment_is_in_progress_and_must_be_resolved_first",
			)
		})
	}
}

func TestCancelByCustomer_RefusesConfirmingSessionEvenAfterTTL(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	// A confirming session past expires_at is still owned by Confirm.
	confirming := h.session(
		withStatus(model.ConfirmingCheckoutSessionStatus),
		withExpiresAt(h.now.Add(-time.Minute)),
	)

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.expectCustomerLock()

	_, err := h.uc.CancelByCustomer(h.ctx, CancelByCustomerInput{
		CheckoutSessionID: h.sessionID,
		IdentitySubjectID: h.profileID,
	})
	requireCerr(
		t,
		err,
		cerr.CodeConflict,
		"error.checkout_payment_is_in_progress_and_must_be_resolved_first",
	)
}

func TestCancelByCustomer_AfterTTLExpiresSessionInsteadOfCanceling(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	pastDue := h.session(withExpiresAt(h.now.Add(-time.Minute)))
	expired := h.session(
		withStatus(model.ExpiredCheckoutSessionStatus),
		withExpiresAt(h.now.Add(-time.Minute)),
	)

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(pastDue, nil).Once()
	h.expectCustomerLock()

	// The only status write is open -> expired; MarkCanceled is never called.
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, h.now).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	output, err := h.uc.CancelByCustomer(h.ctx, CancelByCustomerInput{
		CheckoutSessionID: h.sessionID,
		IdentitySubjectID: h.profileID,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
	require.Empty(t, output.ReturnURL)
	require.Equal(t, model.CheckoutSession{}, output.Session)
}

func TestCancelByCustomer_OpenSessionIsCanceledWithCanceledEvent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	open := h.session()
	canceled := h.session(withStatus(model.CanceledCheckoutSessionStatus))

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectCustomerLock()
	h.sessionTx.EXPECT().
		MarkCanceled(h.ctx, h.sessionID, model.CustomerCanceledCheckoutReason, h.now).
		Return(canceled, nil).Once()
	h.expectLifecycleEvent(canceled, checkoutlifecycle.EventTypeCanceled)

	output, err := h.uc.CancelByCustomer(h.ctx, CancelByCustomerInput{
		CheckoutSessionID: h.sessionID,
		IdentitySubjectID: h.profileID,
	})
	require.NoError(t, err)
	require.Equal(t, model.CanceledCheckoutSessionStatus, output.Session.Status)
	require.Contains(t, output.ReturnURL, "checkout_session_id="+h.publicToken.String())
}
