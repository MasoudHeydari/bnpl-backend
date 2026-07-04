package checkoutsessionuc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestConfirmExternal_VerifiedProcessingReplaysWithoutRechallengingOTP(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// verify Tx + Confirm claim Tx + payment-attempt Tx; contract/policy are Conn-only.
	h.expectPool(2, 3)

	verified := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
		withOTPVerifiedAt(h.now.Add(-time.Minute)),
	)
	prepared := h.createdContract(true)
	prepared.Contract.DecisionID = h.decisionID

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(verified, nil).Once()
	h.customerTx.EXPECT().LockByID(h.ctx, h.customerID).Return(h.customerProfile(), nil).Once()

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(verified, nil).Once()
	h.expectCustomerLock()
	h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(prepared.Contract, nil).Once()
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		PolicyID: "checkout_contract_approval", PolicyVersion: "v1",
	}, nil).Once()
	h.expectCheckoutPaymentAttemptLoad(true)

	output, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	require.NoError(t, err)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, output.Session.Status)
	require.Equal(t, h.contractID, output.Contract.ID)
	require.True(t, output.PaymentRequired)
	require.Empty(t, h.contracts.calls)
}

func TestConfirmExternal_VerifiedCompletedReplaysIdempotently(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// verify Tx + Confirm claim Tx; completed contract load is Conn-only.
	h.expectPool(1, 2)

	verified := h.session(
		withStatus(model.CompletedCheckoutSessionStatus),
		withContract(h.contractID),
		withOTPVerifiedAt(h.now.Add(-time.Minute)),
	)
	prepared := h.createdContract(false)
	prepared.Contract.DecisionID = h.decisionID
	prepared.Contract.Status = model.ActiveContractStatus

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(verified, nil).Once()
	h.customerTx.EXPECT().LockByID(h.ctx, h.customerID).Return(h.customerProfile(), nil).Once()

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(verified, nil).Once()
	h.expectCustomerLock()
	h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(prepared.Contract, nil).Once()

	output, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	require.NoError(t, err)
	require.Equal(t, model.CompletedCheckoutSessionStatus, output.Session.Status)
	require.Equal(t, h.contractID, output.Contract.ID)
	require.Empty(t, h.contracts.calls)
}

func TestConfirmExternal_UnverifiedProcessingDoesNotBypassOTP(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	pending := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
		withOTPVerifier(t, h.publicToken, testOTPCode),
	)

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_cannot_continue_in_its_current_state")
}

func TestConfirmExternal_VerifiedTerminalStatesAreRejected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status model.CheckoutSessionStatus
	}{
		{name: "canceled", status: model.CanceledCheckoutSessionStatus},
		{name: "expired", status: model.ExpiredCheckoutSessionStatus},
		{name: "unavailable", status: model.UnavailableCheckoutSessionStatus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.expectPool(0, 1)

			verified := h.session(
				withStatus(tt.status),
				withOTPVerifiedAt(h.now.Add(-time.Minute)),
			)

			h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(verified, nil).Once()

			_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
			requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_cannot_continue_in_its_current_state")
		})
	}
}
