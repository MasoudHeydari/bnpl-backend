package checkoutsessionuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

func TestConfirm_MatchingGenerationClaimsSessionAndLinksContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		paymentRequired bool
		wantStatus      model.CheckoutSessionStatus
	}{
		{
			name:            "payment required advances to processing",
			paymentRequired: true,
			wantStatus:      model.ProcessingCheckoutSessionStatus,
		},
		{
			name:            "merchant funding completes only after FC recognition",
			paymentRequired: false,
			wantStatus:      model.CompletedCheckoutSessionStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			txCalls := 2
			if !tt.paymentRequired {
				txCalls++ // authoritative FC recognition, then checkout completion
			}
			// beginConfirm TX + markConfirming TX (+ finishConfirm / complete).
			h.expectPool(0, txCalls+1)

			open := h.session()
			confirming := h.session(withStatus(model.ConfirmingCheckoutSessionStatus))
			linked := h.session(withStatus(model.ConfirmingCheckoutSessionStatus), withContract(h.contractID))
			processing := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
			final := h.session(withStatus(tt.wantStatus), withContract(h.contractID))

			// beginConfirm: open session, matching generation, Decision not expired.
			h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
			h.expectCustomerLock()
			h.decisionTx.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID}, nil).Once()

			// After Decision-domain revalidation succeeds, claim confirming.
			h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
			h.sessionTx.EXPECT().MarkConfirming(h.ctx, h.sessionID, h.now).Return(confirming, nil).Once()

			// Link transaction after the contract exists.
			h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
			h.sessionTx.EXPECT().LinkContract(h.ctx, h.sessionID, h.contractID, h.now).Return(linked, nil).Once()
			h.sessionTx.EXPECT().UpdateStatus(h.ctx, h.sessionID, model.ProcessingCheckoutSessionStatus, h.now).
				Return(processing, nil).Once()

			h.contracts.output = h.createdContract(tt.paymentRequired)
			if !tt.paymentRequired {
				// Preparation cannot activate the Contract. Only the subsequent
				// FC recognition may return ACTIVE plus durable entitlement IDs.
				h.contracts.output.Contract.Status = model.PaymentPendingContractStatus
				merchantEntitlementID, receivableID := "fc-entitlement", "fc-receivable"
				recognized := h.createdContract(false)
				recognized.Contract.FinancialMerchantEntitlementID = &merchantEntitlementID
				recognized.Contract.FinancialReceivableID = &receivableID
				h.contracts.submitted = &recognized

				h.contractTx.EXPECT().GetByID(h.ctx, h.contractID).Return(recognized.Contract, nil).Once()
				h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(processing, nil).Once()
				h.sessionTx.EXPECT().UpdateStatus(h.ctx, h.sessionID, model.CompletedCheckoutSessionStatus, h.now).
					Return(final, nil).Once()
				h.expectLifecycleEvent(final, checkoutlifecycle.EventTypeCompleted)
				h.sessionTx.EXPECT().LockByContractID(h.ctx, h.contractID).Return(final, nil).Once()
			}

			output, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
			require.NoError(t, err)
			require.False(t, output.ReconfirmRequired)
			require.Equal(t, tt.wantStatus, output.Session.Status)
			require.Equal(t, h.contractID, output.Contract.ID)
			require.Equal(t, tt.paymentRequired, output.PaymentRequired)

			require.Len(t, h.contracts.calls, 1)
			require.Equal(t, h.decisionID, h.contracts.calls[0].DecisionID)
			require.Equal(t, h.profileID, h.contracts.calls[0].ExternalCustomerID)
		})
	}
}

func TestConfirm_StaleGenerationRequiresReconfirmWithoutCreatingContract(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// Claim attempt, generation CAS bump, and the refreshed evaluation record.
	h.expectPool(1, 3)

	stale := h.session(withGeneration(2))
	expiredAt := h.now.Add(-time.Minute)
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID, ExpiresAt: &expiredAt}, nil).Once()
	bumped := h.session(withGeneration(3), withoutDecision())
	newDecisionID := model.DecisionID(uuid.New())
	relinked := h.session(withGeneration(3), func(s *model.CheckoutSession) { s.DecisionID = &newDecisionID })

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(stale, nil).Once()
	h.expectCustomerLock()

	// Stale presented generation: MarkConfirming must not run (strict mock).
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(stale, nil).Once()
	h.sessionTx.EXPECT().
		BumpEvaluationGenerationCAS(h.ctx, h.sessionID, 2, stale.DecisionID, h.now).
		Return(bumped, nil).Once()

	expiresAt := h.now.Add(15 * time.Minute)
	h.decisions.output = decisionuc.CreateOutput{
		Decision:                    model.Decision{ID: newDecisionID, Outcome: model.ApprovedDecisionOutcome},
		RequestedAmountMinor:        150000000,
		ApprovedFinancedAmountMinor: 150000000,
		Currency:                    model.IRRCurrency,
		ExpiresAt:                   &expiresAt,
	}

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(bumped, nil).Once()
	h.sessionTx.EXPECT().LinkDecision(h.ctx, h.sessionID, newDecisionID, 3, h.now).Return(relinked, nil).Once()

	// The customer presented generation 1, but the session is already at 2.
	output, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.NoError(t, err)
	require.True(t, output.ReconfirmRequired)
	require.Equal(t, ApprovedOfferOutcome, output.Offer.Outcome)
	require.Equal(t, 3, output.Session.EvaluationGeneration)
	require.Equal(t, model.OpenCheckoutSessionStatus, output.Session.Status)
	require.Empty(t, h.contracts.calls, "stale generation must never create a Contract")

	require.Len(t, h.decisions.calls, 1)
	require.Equal(t, checkoutSessionDecisionKey(h.sessionID, 3), h.decisions.calls[0].CheckoutKey)
}

func TestConfirm_StaleGenerationWithDeniedRefreshMarksUnavailableWithoutContract(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 3)

	stale := h.session(withGeneration(2))
	expiredAt := h.now.Add(-time.Minute)
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID, ExpiresAt: &expiredAt}, nil).Once()
	bumped := h.session(withGeneration(3), withoutDecision())
	newDecisionID := model.DecisionID(uuid.New())
	relinked := h.session(withGeneration(3), func(s *model.CheckoutSession) { s.DecisionID = &newDecisionID })
	unavailable := h.session(withGeneration(3), withStatus(model.UnavailableCheckoutSessionStatus), func(s *model.CheckoutSession) {
		s.DecisionID = &newDecisionID
	})

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(stale, nil).Once()
	h.expectCustomerLock()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(stale, nil).Once()
	h.sessionTx.EXPECT().
		BumpEvaluationGenerationCAS(h.ctx, h.sessionID, 2, stale.DecisionID, h.now).
		Return(bumped, nil).Once()

	h.decisions.output = decisionuc.CreateOutput{
		Decision: model.Decision{ID: newDecisionID, Outcome: model.DeniedDecisionOutcome},
	}

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(bumped, nil).Once()
	h.sessionTx.EXPECT().LinkDecision(h.ctx, h.sessionID, newDecisionID, 3, h.now).Return(relinked, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.UnavailableCheckoutSessionStatus, h.now).
		Return(unavailable, nil).Once()
	h.expectLifecycleEvent(unavailable, checkoutlifecycle.EventTypeUnavailable)

	_, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_unavailable")
	require.Empty(t, h.contracts.calls)
}

func TestConfirm_RetryWhileConfirmingWithContractResumesLinkageAndPayment(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// beginConfirm TX, load contract Conn, policy Conn, load attempt TX, finishConfirm TX.
	h.expectPool(2, 3)

	confirming := h.session(withStatus(model.ConfirmingCheckoutSessionStatus), withContract(h.contractID))
	processing := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	prepared := h.createdContract(true)
	prepared.Contract.DecisionID = h.decisionID

	// Begin: already confirming with a linked contract, so no MarkConfirming.
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.expectCustomerLock()

	h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(prepared.Contract, nil).Once()
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		PolicyID: "checkout_contract_approval", PolicyVersion: "v1",
	}, nil).Once()
	h.expectCheckoutPaymentAttemptLoad(true)

	// finishConfirm advances confirming → processing without re-linking.
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.sessionTx.EXPECT().UpdateStatus(h.ctx, h.sessionID, model.ProcessingCheckoutSessionStatus, h.now).
		Return(processing, nil).Once()

	output, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.NoError(t, err)
	require.Equal(t, h.contractID, output.Contract.ID)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, output.Session.Status)
	require.True(t, output.PaymentRequired)
	require.Empty(t, h.contracts.calls, "replay must not prepare a second Contract")
}

func TestConfirm_RetryWhileConfirmingWithoutContractIsInProgress(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	confirming := h.session(withStatus(model.ConfirmingCheckoutSessionStatus))

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.expectCustomerLock()

	_, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
	require.Empty(t, h.contracts.calls)
}

func TestConfirm_RetryWhileConfirmingWithoutContractStaleGenerationIsInProgress(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	confirming := h.session(withStatus(model.ConfirmingCheckoutSessionStatus), withGeneration(2))

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.expectCustomerLock()

	_, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
	require.Empty(t, h.contracts.calls)
}

func TestConfirm_RetryWhileConfirmingWithoutDecisionOrContractIsRejected(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	confirming := h.session(withStatus(model.ConfirmingCheckoutSessionStatus), withoutDecision())

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.expectCustomerLock()

	_, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_payment_is_in_progress_and_must_be_resolved_first")
	require.Empty(t, h.contracts.calls)
}

func TestConfirm_RetryAfterProcessingOrCompletedReplaysExistingContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		status          model.CheckoutSessionStatus
		paymentRequired bool
		connOnly        int
		txCalls         int
		policyVersion   string
	}{
		{
			name:            "processing resumes payment submission",
			status:          model.ProcessingCheckoutSessionStatus,
			paymentRequired: true,
			connOnly:        2, // contract load + policy resolve
			txCalls:         2,
			policyVersion:   "v1",
		},
		{
			name:            "completed is read-only",
			status:          model.CompletedCheckoutSessionStatus,
			paymentRequired: false,
			connOnly:        1,
			txCalls:         1,
			policyVersion:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.expectPool(tt.connOnly, tt.txCalls)

			existing := h.session(withStatus(tt.status), withContract(h.contractID))
			prepared := h.createdContract(tt.paymentRequired)
			prepared.Contract.DecisionID = h.decisionID

			h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(existing, nil).Once()
			h.expectCustomerLock()

			h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(prepared.Contract, nil).Once()
			if tt.policyVersion != "" {
				h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
					PolicyID: "checkout_contract_approval", PolicyVersion: tt.policyVersion,
				}, nil).Once()
			}

			h.expectCheckoutPaymentAttemptLoad(tt.paymentRequired)

			output, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
			require.NoError(t, err)
			require.False(t, output.ReconfirmRequired)
			require.Equal(t, tt.status, output.Session.Status)
			require.Equal(t, h.contractID, output.Contract.ID)
			require.Equal(t, tt.paymentRequired, output.PaymentRequired)
			require.Empty(t, h.contracts.calls)
		})
	}
}

func TestConfirm_RetryByAnotherCustomerIsRejected(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	other := model.CustomerID(uuid.New())
	existing := h.session(
		withStatus(model.CompletedCheckoutSessionStatus),
		withContract(h.contractID),
		withCustomer(&other),
	)

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(existing, nil).Once()
	h.expectCustomerLock()

	_, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	requireCerr(
		t,
		err,
		cerr.CodeForbidden,
		"error.checkout_customer_identity_conflict",
	)
	require.Empty(t, h.contracts.calls)
}

func TestConfirm_ContractFailureReleasesConfirmingClaim(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 2)

	open := h.session()
	confirming := h.session(withStatus(model.ConfirmingCheckoutSessionStatus))

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectCustomerLock()
	h.decisionTx.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID}, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().MarkConfirming(h.ctx, h.sessionID, h.now).Return(confirming, nil).Once()

	// Ambiguous preparation failures cannot release the confirmation claim.
	h.contracts.err = cerr.ServiceUnavailable(errFinancialCoreDown)

	_, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeServiceUnavailable))
	require.Len(t, h.contracts.calls, 1)
}

// Merchant funding never emits COMPLETED or merchant delivery on an ambiguous
// Financial Core error, even though no PSP payment is required.
func TestConfirm_MerchantFundingFinancialCoreUnavailableStaysProcessing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.expectPool(0, 3)

	open := h.session()
	confirming := h.session(withStatus(model.ConfirmingCheckoutSessionStatus))
	linked := h.session(withStatus(model.ConfirmingCheckoutSessionStatus), withContract(h.contractID))
	processing := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectCustomerLock()
	h.decisionTx.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID}, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().MarkConfirming(h.ctx, h.sessionID, h.now).Return(confirming, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.sessionTx.EXPECT().LinkContract(h.ctx, h.sessionID, h.contractID, h.now).Return(linked, nil).Once()
	h.sessionTx.EXPECT().UpdateStatus(h.ctx, h.sessionID, model.ProcessingCheckoutSessionStatus, h.now).
		Return(processing, nil).Once()

	h.contracts.output = h.createdContract(false)
	h.contracts.output.Contract.Status = model.PaymentPendingContractStatus
	h.contracts.submitErr = cerr.ServiceUnavailable(errFinancialCoreDown)

	_, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeServiceUnavailable))
	// There is intentionally no expected call to emit completed or to change
	// PROCESSING to a terminal status.
}

// A normal payment's ACTIVE Contract must not trigger RecordMerchantEntitlement
// simply because processing has not yet received its completion event.
func TestConfirm_ReplayNormalActiveDoesNotSubmitDirectEntitlement(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.expectPool(2, 1) // claim transaction, persisted Contract, policy lookup
	processing := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	contract := h.createdContract(false).Contract
	contract.DecisionID = h.decisionID
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(processing, nil).Once()
	h.expectCustomerLock()
	h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(contract, nil).Once()
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		PolicyID: "checkout_contract_approval", PolicyVersion: "v1",
	}, nil).Once()
	h.contracts.submitErr = errFinancialCoreDown // must never be called
	out, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.NoError(t, err)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, out.Session.Status)
	require.Empty(t, h.contracts.calls)
}
