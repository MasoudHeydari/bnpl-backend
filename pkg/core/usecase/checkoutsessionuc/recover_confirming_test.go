package checkoutsessionuc

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// No committed Contract does not prove the owner is dead: the old worker may
// still be preparing one in an uncommitted transaction or before entering it.
func TestRecoverStaleConfirming_DoesNotReleaseUnfencedClaim(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.expectPool(0, 1)
	confirming := h.confirmingSession()

	h.sessionTx.EXPECT().ReopenStaleConfirmingWithoutContract(
		h.ctx, h.now, h.now.Add(-DefaultConfirmingClaimTimeout), h.uc.expiryBatchSize,
	).Return([]model.CheckoutSession{confirming}, nil).Once()
	h.contractTx.EXPECT().GetByDecisionID(h.ctx, h.decisionID).
		Return(model.Contract{}, cerr.ResourceNotFound(errors.New("contract not yet committed"))).Once()

	output, err := h.uc.RecoverStaleConfirmingSessions(h.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, output.UnresolvedCount)
	require.Zero(t, output.RelinkedCount)
	require.Zero(t, output.ResumedCount)
	// No UpdateStatus(OPEN) and no second Contract preparation are expected.
	require.Empty(t, h.contracts.calls)
}

func TestRecoverStaleConfirming_RelinksAndFinalizesRecognizedContract(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.expectPool(1, 2) // relink TX, policy read, separate completion TX

	confirming := h.confirmingSession()
	linked := h.confirmingSession(withContract(h.contractID))
	processing := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	completed := h.session(withStatus(model.CompletedCheckoutSessionStatus), withContract(h.contractID))

	entitlementID, receivableID := "fc-entitlement", "fc-receivable"
	recognized := h.createdContract(false)
	recognized.Contract.DecisionID = h.decisionID
	recognized.Contract.FinancialMerchantEntitlementID = &entitlementID
	recognized.Contract.FinancialReceivableID = &receivableID

	h.sessionTx.EXPECT().ReopenStaleConfirmingWithoutContract(
		h.ctx, h.now, h.now.Add(-DefaultConfirmingClaimTimeout), h.uc.expiryBatchSize,
	).Return([]model.CheckoutSession{confirming}, nil).Once()
	h.contractTx.EXPECT().GetByDecisionID(h.ctx, h.decisionID).
		Return(recognized.Contract, nil).Once()
	h.sessionTx.EXPECT().LinkContract(h.ctx, h.sessionID, h.contractID, h.now).
		Return(linked, nil).Once()
	h.sessionTx.EXPECT().UpdateStatus(h.ctx, h.sessionID, model.ProcessingCheckoutSessionStatus, h.now).
		Return(processing, nil).Once()

	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		PolicyID: "checkout_contract_approval", PolicyVersion: "v2",
	}, nil).Once()
	h.contractTx.EXPECT().GetByID(h.ctx, h.contractID).Return(recognized.Contract, nil).Once()
	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(processing, nil).Once()
	h.sessionTx.EXPECT().UpdateStatus(h.ctx, h.sessionID, model.CompletedCheckoutSessionStatus, h.now).
		Return(completed, nil).Once()
	h.expectLifecycleEvent(completed, checkoutlifecycle.EventTypeCompleted)
	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(completed, nil).Once()

	output, err := h.uc.RecoverStaleConfirmingSessions(h.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, output.RelinkedCount)
	require.Equal(t, 1, output.ResumedCount)
	require.Zero(t, output.UnresolvedCount)
}

// A crash after durable linkage but before/outside FC recognition must be
// resumable without requiring the customer to re-enter the hosted page.
func TestRecoverStaleConfirming_ResumesLinkedActivationPending(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.expectPool(1, 2)
	processing := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	completed := h.session(withStatus(model.CompletedCheckoutSessionStatus), withContract(h.contractID))
	pending := h.createdContract(false)
	pending.Contract.DecisionID = h.decisionID
	pending.Contract.Status = model.PaymentPendingContractStatus
	entitlementID, receivableID := "fc-entitlement", "fc-receivable"
	recognized := h.createdContract(false)
	recognized.Contract.DecisionID = h.decisionID
	recognized.Contract.FinancialMerchantEntitlementID = &entitlementID
	recognized.Contract.FinancialReceivableID = &receivableID
	h.contracts.submitted = &recognized

	h.sessionTx.EXPECT().ReopenStaleConfirmingWithoutContract(
		h.ctx, h.now, h.now.Add(-DefaultConfirmingClaimTimeout), h.uc.expiryBatchSize,
	).Return([]model.CheckoutSession{processing}, nil).Once()
	h.contractTx.EXPECT().GetByID(h.ctx, h.contractID).Return(pending.Contract, nil).Once()
	// Refresh the recovery timestamp under the SKIP LOCKED row lock.
	h.sessionTx.EXPECT().UpdateStatus(h.ctx, h.sessionID, model.ProcessingCheckoutSessionStatus, h.now).
		Return(processing, nil).Once()

	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		PolicyID: "checkout_contract_approval", PolicyVersion: "v2",
	}, nil).Once()
	h.contractTx.EXPECT().GetByID(h.ctx, h.contractID).Return(recognized.Contract, nil).Once()
	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(processing, nil).Once()
	h.sessionTx.EXPECT().UpdateStatus(h.ctx, h.sessionID, model.CompletedCheckoutSessionStatus, h.now).
		Return(completed, nil).Once()
	h.expectLifecycleEvent(completed, checkoutlifecycle.EventTypeCompleted)
	h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(completed, nil).Once()

	output, err := h.uc.RecoverStaleConfirmingSessions(h.ctx)
	require.NoError(t, err)
	require.Zero(t, output.RelinkedCount)
	require.Equal(t, 1, output.ResumedCount)
}

// A normal payment-based Contract is never retried through the trusted-user
// entitlement path. Its terminal payment state belongs to the FC result
// consumer/reconciliation even if it already has accounting identifiers.
func TestRecoverStaleConfirming_ActiveNormalPaymentIsNotMerchantFunding(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.expectPool(1, 1) // scan TX and immutable Decision policy read
	processing := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	active := h.createdContract(false).Contract
	active.DecisionID = h.decisionID
	entitlementID, receivableID := "payment-entitlement", "payment-receivable"
	active.FinancialMerchantEntitlementID = &entitlementID
	active.FinancialReceivableID = &receivableID

	h.sessionTx.EXPECT().ReopenStaleConfirmingWithoutContract(
		h.ctx, h.now, h.now.Add(-DefaultConfirmingClaimTimeout), h.uc.expiryBatchSize,
	).Return([]model.CheckoutSession{processing}, nil).Once()
	h.contractTx.EXPECT().GetByID(h.ctx, h.contractID).Return(active, nil).Once()
	h.sessionTx.EXPECT().UpdateStatus(h.ctx, h.sessionID, model.ProcessingCheckoutSessionStatus, h.now).
		Return(processing, nil).Once()
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		PolicyID: "checkout_contract_approval", PolicyVersion: "v1",
	}, nil).Once()

	output, err := h.uc.RecoverStaleConfirmingSessions(h.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, output.UnresolvedCount)
	require.Zero(t, output.ResumedCount)
	require.Empty(t, h.contracts.calls)
}
