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

func TestConfirm_RevalidationHonorableContinuesWithoutGenerationBump(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 3)

	open := h.session()
	linked := h.confirmingSession(withContract(h.contractID))
	processing := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))

	h.decisions.revalidateOutput = decisionuc.RevalidateCheckoutOfferOutput{StillHonorable: true}

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectCustomerLock()
	h.decisionTx.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID}, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectMarkConfirming()
	confirming := h.confirmingSession()
	h.expectConfirmClaimStepLock(confirming)

	prepared := h.createdContract(true)
	prepared.Contract.DecisionID = h.decisionID
	h.contracts.output = prepared

	h.expectConfirmSagaDriveLock(confirming)
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.sessionTx.EXPECT().LinkContract(h.ctx, h.sessionID, h.contractID, h.now).Return(linked, nil).Once()
	h.sessionTx.EXPECT().UpdateStatus(h.ctx, h.sessionID, model.ProcessingCheckoutSessionStatus, h.now).
		Return(processing, nil).Once()

	h.expectConfirmProjectionLock(processing)
	h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(prepared.Contract, nil).Once()
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		PolicyID: "checkout_contract_approval", PolicyVersion: "v1",
	}, nil).Once()
	h.expectCheckoutPaymentAttemptLoad(true)

	output, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.NoError(t, err)
	require.False(t, output.ReconfirmRequired)
	require.Equal(t, 1, output.Session.EvaluationGeneration)
	require.Equal(t, h.decisionID, *output.Session.DecisionID)
	require.Len(t, h.decisions.revalidateCalls, 1)
	require.Empty(t, h.decisions.calls, "honorable revalidation must not Create a new Decision")
	require.Len(t, h.contracts.calls, 1)
}

func TestConfirm_MaterialChangeRequiresReconfirmWithoutContractOrFC(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// beginConfirm + force bump + recordEvaluation (clamp skipped: offer TTL later).
	h.expectPool(0, 3)

	open := h.session()
	bumped := h.session(withGeneration(2), withoutDecision())
	newDecisionID := model.DecisionID(uuid.New())
	relinked := h.session(withGeneration(2), func(s *model.CheckoutSession) {
		s.DecisionID = &newDecisionID
	})

	h.decisions.revalidateOutput = decisionuc.RevalidateCheckoutOfferOutput{StillHonorable: false}
	expiresAt := h.now.Add(15 * time.Minute)
	h.decisions.output = decisionuc.CreateOutput{
		Decision:                    model.Decision{ID: newDecisionID, Outcome: model.ApprovedDecisionOutcome},
		RequestedAmountMinor:        150000000,
		ApprovedFinancedAmountMinor: 120000000,
		UpfrontAmountMinor:          30000000,
		Currency:                    model.IRRCurrency,
		ExpiresAt:                   &expiresAt,
	}

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectCustomerLock()
	h.decisionTx.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID}, nil).Once()

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().
		BumpEvaluationGenerationCAS(h.ctx, h.sessionID, 1, open.DecisionID, h.now).
		Return(bumped, nil).Once()

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(bumped, nil).Once()
	h.sessionTx.EXPECT().LinkDecision(h.ctx, h.sessionID, newDecisionID, 2, h.now).Return(relinked, nil).Once()

	output, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.NoError(t, err)
	require.True(t, output.ReconfirmRequired)
	require.Equal(t, 2, output.Session.EvaluationGeneration)
	require.Equal(t, ApprovedOfferOutcome, output.Offer.Outcome)
	require.Equal(t, uint64(120000000), output.Offer.ApprovedFinancedAmountMinor)
	require.Empty(t, h.contracts.calls)
	require.Len(t, h.decisions.calls, 1)
	require.Equal(t, checkoutSessionDecisionKey(h.sessionID, 2), h.decisions.calls[0].CheckoutKey)
}

func TestConfirm_MaterialChangeDeniedMarksUnavailable(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 3)

	open := h.session()
	bumped := h.session(withGeneration(2), withoutDecision())
	newDecisionID := model.DecisionID(uuid.New())
	relinked := h.session(withGeneration(2), func(s *model.CheckoutSession) {
		s.DecisionID = &newDecisionID
	})
	unavailable := h.session(withGeneration(2), withStatus(model.UnavailableCheckoutSessionStatus), func(s *model.CheckoutSession) {
		s.DecisionID = &newDecisionID
	})

	h.decisions.revalidateOutput = decisionuc.RevalidateCheckoutOfferOutput{StillHonorable: false}
	h.decisions.output = decisionuc.CreateOutput{
		Decision: model.Decision{ID: newDecisionID, Outcome: model.DeniedDecisionOutcome},
	}

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectCustomerLock()
	h.decisionTx.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID}, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().
		BumpEvaluationGenerationCAS(h.ctx, h.sessionID, 1, open.DecisionID, h.now).
		Return(bumped, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(bumped, nil).Once()
	h.sessionTx.EXPECT().LinkDecision(h.ctx, h.sessionID, newDecisionID, 2, h.now).Return(relinked, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.UnavailableCheckoutSessionStatus, h.now).
		Return(unavailable, nil).Once()
	h.expectLifecycleEvent(unavailable, checkoutlifecycle.EventTypeUnavailable)

	_, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_unavailable")
	require.Empty(t, h.contracts.calls)
}

func TestConfirm_DeterministicInsufficientCreditRefreshesOffer(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// begin + claim + reopen + bump (no immediate re-evaluate; unavailable is terminal)
	h.expectPool(0, 4)

	open := h.session()
	reopened := h.session()
	bumped := h.session(withGeneration(2), withoutDecision())

	h.decisions.revalidateOutput = decisionuc.RevalidateCheckoutOfferOutput{StillHonorable: true}

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectCustomerLock()
	h.decisionTx.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID}, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectMarkConfirming()
	confirming := h.confirmingSession()
	h.expectConfirmClaimStepLock(confirming)
	h.expectConfirmSagaDriveLock(confirming)

	h.contracts.err = cerr.NewPublic(
		cerr.CodeConflict,
		"error.insufficient_credit_for_contract",
		nil,
		cerr.Details{Violations: []cerr.Violation{{Code: "INSUFFICIENT_CREDIT"}}},
	)

	h.expectConfirmProjectionLock(confirming)
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(confirming, nil).Once()
	h.contractTx.EXPECT().GetByDecisionID(h.ctx, h.decisionID).
		Return(model.Contract{}, cerr.ResourceNotFound(nil)).Once()
	h.sessionTx.EXPECT().
		ReopenConfirmingToOpen(h.ctx, h.sessionID, h.confirmationAttemptID, h.now).
		Return(reopened, nil).Once()

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(reopened, nil).Once()
	h.sessionTx.EXPECT().
		BumpEvaluationGenerationCAS(h.ctx, h.sessionID, 1, reopened.DecisionID, h.now).
		Return(bumped, nil).Once()

	output, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.NoError(t, err)
	require.True(t, output.ReconfirmRequired)
	require.Equal(t, 2, output.Session.EvaluationGeneration)
	require.Equal(t, model.OpenCheckoutSessionStatus, output.Session.Status)
	require.Nil(t, output.Session.DecisionID)
	require.Len(t, h.contracts.calls, 1)
	require.Empty(t, h.decisions.calls, "post-compensation refresh must not evaluate under the failing credit state")
}

func TestConfirm_AmbiguousContractFailureDoesNotReopen(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 2)

	open := h.session()

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectCustomerLock()
	h.decisionTx.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID}, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectMarkConfirming()
	confirming := h.confirmingSession()
	h.expectConfirmClaimStepLock(confirming)
	h.expectConfirmSagaDriveLock(confirming)

	h.contracts.err = cerr.ServiceUnavailable(errFinancialCoreDown)

	_, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeServiceUnavailable))
	require.Len(t, h.contracts.calls, 1)
}

func TestMarkConfirmingOpenSession_ExpiredAfterRevalidationPersistsExpiry(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	open := h.session(withExpiresAt(h.now))
	expired := h.session(withExpiresAt(h.now), withStatus(model.ExpiredCheckoutSessionStatus))

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, h.now).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	_, err := h.uc.markConfirmingOpenSession(h.ctx, open)
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
}

func TestIsDeterministicOfferNoLongerHonorable_AllocationExpired(t *testing.T) {
	t.Parallel()

	err := cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_decision_allocation_expired",
		nil,
		cerr.Details{
			Violations: []cerr.Violation{{
				Code: "CHECKOUT_DECISION_ALLOCATION_EXPIRED",
			}},
		},
	)

	require.True(t, isDeterministicOfferNoLongerHonorable(err))
}

func TestConfirm_ReconfirmDoesNotExtendCheckoutExpiresAt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// begin + bump + recordEvaluation (clamp no-ops when session deadline is earlier)
	h.expectPool(0, 3)

	sessionDeadline := h.now.Add(2 * time.Minute)
	open := h.session(withExpiresAt(sessionDeadline))
	bumped := h.session(withGeneration(2), withoutDecision(), withExpiresAt(sessionDeadline))
	newDecisionID := model.DecisionID(uuid.New())
	relinked := h.session(withGeneration(2), withExpiresAt(sessionDeadline), func(s *model.CheckoutSession) {
		s.DecisionID = &newDecisionID
	})
	// Clamp LEAST keeps the original session deadline even when Decision TTL is later.
	clamped := relinked

	h.decisions.revalidateOutput = decisionuc.RevalidateCheckoutOfferOutput{StillHonorable: false}
	laterDecisionExpiry := h.now.Add(15 * time.Minute)
	h.decisions.output = decisionuc.CreateOutput{
		Decision:                    model.Decision{ID: newDecisionID, Outcome: model.ApprovedDecisionOutcome},
		RequestedAmountMinor:        150000000,
		ApprovedFinancedAmountMinor: 150000000,
		Currency:                    model.IRRCurrency,
		ExpiresAt:                   &laterDecisionExpiry,
	}

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectCustomerLock()
	h.decisionTx.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID}, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().
		BumpEvaluationGenerationCAS(h.ctx, h.sessionID, 1, open.DecisionID, h.now).
		Return(bumped, nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(bumped, nil).Once()
	h.sessionTx.EXPECT().LinkDecision(h.ctx, h.sessionID, newDecisionID, 2, h.now).Return(relinked, nil).Once()

	// Offer nominal expiry is later than session deadline → ClampExpiresAt is a no-op
	// at the use-case layer (session already earlier), so no ClampExpiresAt call.
	output, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.NoError(t, err)
	require.True(t, output.ReconfirmRequired)
	require.Equal(t, sessionDeadline, output.Session.ExpiresAt)
	require.Equal(t, clamped.ExpiresAt, output.Session.ExpiresAt)
}

func TestConfirm_ConcurrentGenerationBumpReusesWinner(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 3)

	open := h.session()
	winner := h.session(withGeneration(2), withoutDecision())
	newDecisionID := model.DecisionID(uuid.New())
	relinked := h.session(withGeneration(2), func(s *model.CheckoutSession) {
		s.DecisionID = &newDecisionID
	})

	h.decisions.revalidateOutput = decisionuc.RevalidateCheckoutOfferOutput{StillHonorable: false}
	expiresAt := h.now.Add(15 * time.Minute)
	h.decisions.output = decisionuc.CreateOutput{
		Decision:                    model.Decision{ID: newDecisionID, Outcome: model.ApprovedDecisionOutcome},
		RequestedAmountMinor:        150000000,
		ApprovedFinancedAmountMinor: 150000000,
		Currency:                    model.IRRCurrency,
		ExpiresAt:                   &expiresAt,
	}

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.expectCustomerLock()
	h.decisionTx.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{ID: h.decisionID}, nil).Once()

	// CAS loses to a concurrent bumper; reload winner and continue without a second bump.
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(open, nil).Once()
	h.sessionTx.EXPECT().
		BumpEvaluationGenerationCAS(h.ctx, h.sessionID, 1, open.DecisionID, h.now).
		Return(model.CheckoutSession{}, cerr.Conflict(nil)).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(winner, nil).Once()

	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(winner, nil).Once()
	h.sessionTx.EXPECT().LinkDecision(h.ctx, h.sessionID, newDecisionID, 2, h.now).Return(relinked, nil).Once()

	output, err := h.uc.Confirm(h.ctx, h.confirmInput(1))
	require.NoError(t, err)
	require.True(t, output.ReconfirmRequired)
	require.Equal(t, 2, output.Session.EvaluationGeneration)
	require.Len(t, h.decisions.calls, 1)
}
