package checkoutsessionuc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestConfirmExternal_ConsumedOTPNeverReplaysPublicConfirm(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	consumed := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
	)
	h.seedOTPChallenge(consumed, testOTPCode, withOTPVerifiedAt(h.now.Add(-time.Minute)))

	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(consumed, nil).Once()
	h.expectOTPContinuationDeps(consumed)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(consumed, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.otp_already_consumed")
	require.Empty(t, h.contracts.calls)
}

func TestConfirmExternal_FreshOTPOnProcessingRunsConfirmCore(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(2, 3)

	pending := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
	)
	h.seedOTPChallenge(pending, testOTPCode)

	verified := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
	)
	prepared := h.createdContract(true)
	prepared.Contract.DecisionID = h.decisionID

	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	// Pre-consume eligibility + ConsumeAuthorizer re-assert under held locks.
	h.expectOTPContinuationDeps(pending)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.expectOTPContinuationDeps(pending)
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

func TestConfirmExternal_CompletedRejectsEvenWithFreshOTP(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	pending := h.session(
		withStatus(model.CompletedCheckoutSessionStatus),
		withContract(h.contractID),
	)
	h.seedOTPChallenge(pending, testOTPCode)

	// Terminal statuses are rejected after peek — no FOR UPDATE.
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_cannot_continue_in_its_current_state")
}

func TestConfirmExternal_TerminalStatesAreRejected(t *testing.T) {
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

			session := h.session(withStatus(tt.status))
			h.seedOTPChallenge(session, testOTPCode)

			h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

			_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
			requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_cannot_continue_in_its_current_state")
		})
	}
}
