package checkoutsessionuc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCancelByToken_RefusesSessionOwnedByConfirmOrPayment(t *testing.T) {
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
			h.expectPool(1, 1)

			session := h.session(withStatus(tt.status))
			h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
			h.sessionTx.EXPECT().
				LockByID(h.ctx, h.sessionID).
				Return(session, nil).Once()

			_, err := h.uc.CancelByToken(h.ctx, CancelByTokenInput{Token: h.publicToken})
			requireCerr(
				t,
				err,
				cerr.CodeConflict,
				"error.checkout_payment_is_in_progress_and_must_be_resolved_first",
			)
		})
	}
}

func TestCancelByToken_RefusesConfirmingSessionEvenAfterTTL(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	confirming := h.session(
		withStatus(model.ConfirmingCheckoutSessionStatus),
		withExpiresAt(h.now.Add(-time.Minute)),
	)

	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(confirming, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()

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
