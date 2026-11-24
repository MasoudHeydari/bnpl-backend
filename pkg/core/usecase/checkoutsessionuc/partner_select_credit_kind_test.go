package checkoutsessionuc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestSelectCreditKind_FirstSelectionDoesNotBumpGeneration(t *testing.T) {
	h := newHarness(t)
	h.expectPool(0, 1)
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	open := h.session(func(s *model.CheckoutSession) {
		s.CreditKind = ""
		s.DecisionID = nil
		s.OfferPreparationMode = model.OfferPreparationOnStart
	})
	selected := open
	selected.CreditKind = model.PayInInstallmentsCreditKind

	h.sessionCQ.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
		Return(open, nil).Once()
	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).
		Return(open, nil).Once()
	h.merchantTx.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()
	h.sessionTx.EXPECT().
		ReplaceCreditKindAndInvalidateEvaluation(
			h.ctx,
			h.sessionID,
			model.PayInInstallmentsCreditKind,
			1,
			(*model.DecisionID)(nil),
			h.now,
		).
		Return(selected, nil).Once()

	output, err := h.uc.SelectCreditKind(h.ctx, SelectCreditKindInput{
		Token:                        h.publicToken,
		ExternalMerchantID:           h.merchantID,
		CreditKind:                   model.PayInInstallmentsCreditKind,
		ExpectedEvaluationGeneration: 1,
	})
	require.NoError(t, err)
	require.Equal(t, model.PayInInstallmentsCreditKind, output.Session.CreditKind)
	require.Equal(t, 1, output.Session.EvaluationGeneration)
	require.Nil(t, output.Offer)
}

func TestSelectCreditKind_SameKindIsIdempotent(t *testing.T) {
	h := newHarness(t)
	h.expectPool(0, 1)
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	open := h.session(func(s *model.CheckoutSession) {
		s.CreditKind = model.PayInInstallmentsCreditKind
		s.OfferPreparationMode = model.OfferPreparationOnStart
	})

	h.sessionCQ.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
		Return(open, nil).Once()
	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).
		Return(open, nil).Once()
	h.merchantTx.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	output, err := h.uc.SelectCreditKind(h.ctx, SelectCreditKindInput{
		Token:                        h.publicToken,
		ExternalMerchantID:           h.merchantID,
		CreditKind:                   model.PayInInstallmentsCreditKind,
		ExpectedEvaluationGeneration: 99, // stale expected still OK when kind matches
	})
	require.NoError(t, err)
	require.Equal(t, open.CreditKind, output.Session.CreditKind)
	require.Equal(t, open.EvaluationGeneration, output.Session.EvaluationGeneration)
	require.Equal(t, open.DecisionID, output.Session.DecisionID)
}

func TestSelectCreditKind_ReplaceApprovedDecisionBumpsOnce(t *testing.T) {
	h := newHarness(t)
	h.expectPool(0, 1)
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	decisionID := h.decisionID
	open := h.session(func(s *model.CheckoutSession) {
		// Start from monthly so replacement targets the always-enabled installment kind.
		s.CreditKind = model.PayNextMonthCreditKind
		s.DecisionID = &decisionID
		s.EvaluationGeneration = 1
		s.OfferPreparationMode = model.OfferPreparationOnStart
	})
	replaced := open
	replaced.CreditKind = model.PayInInstallmentsCreditKind
	replaced.DecisionID = nil
	replaced.EvaluationGeneration = 2

	h.sessionCQ.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
		Return(open, nil).Once()
	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).
		Return(open, nil).Once()
	h.merchantTx.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()
	h.sessionTx.EXPECT().
		ReplaceCreditKindAndInvalidateEvaluation(
			h.ctx,
			h.sessionID,
			model.PayInInstallmentsCreditKind,
			1,
			&decisionID,
			h.now,
		).
		Return(replaced, nil).Once()

	output, err := h.uc.SelectCreditKind(h.ctx, SelectCreditKindInput{
		Token:                        h.publicToken,
		ExternalMerchantID:           h.merchantID,
		CreditKind:                   model.PayInInstallmentsCreditKind,
		ExpectedEvaluationGeneration: 1,
	})
	require.NoError(t, err)
	require.Equal(t, model.PayInInstallmentsCreditKind, output.Session.CreditKind)
	require.Equal(t, 2, output.Session.EvaluationGeneration)
	require.Nil(t, output.Session.DecisionID)
}

func TestSelectCreditKind_StaleExpectedGenerationConflicts(t *testing.T) {
	h := newHarness(t)
	h.expectPool(0, 1)
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	open := h.session(func(s *model.CheckoutSession) {
		s.CreditKind = model.PayNextMonthCreditKind
		s.EvaluationGeneration = 2
		s.OfferPreparationMode = model.OfferPreparationOnStart
	})

	h.sessionCQ.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
		Return(open, nil).Once()
	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).
		Return(open, nil).Once()
	h.merchantTx.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	_, err := h.uc.SelectCreditKind(h.ctx, SelectCreditKindInput{
		Token:                        h.publicToken,
		ExternalMerchantID:           h.merchantID,
		CreditKind:                   model.PayInInstallmentsCreditKind,
		ExpectedEvaluationGeneration: 1,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_credit_kind_selection_stale")
}

func TestSelectCreditKind_RejectsConfirmingSession(t *testing.T) {
	h := newHarness(t)
	h.expectPool(0, 1)
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	confirming := h.confirmingSession()

	h.sessionCQ.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
		Return(confirming, nil).Once()
	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).
		Return(confirming, nil).Once()
	h.merchantTx.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	_, err := h.uc.SelectCreditKind(h.ctx, SelectCreditKindInput{
		Token:                        h.publicToken,
		ExternalMerchantID:           h.merchantID,
		CreditKind:                   model.PayInInstallmentsCreditKind,
		ExpectedEvaluationGeneration: 1,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_cannot_continue_in_its_current_state")
}

func TestSelectCreditKind_RejectsActiveConfirmationOwnership(t *testing.T) {
	h := newHarness(t)
	h.expectPool(0, 1)
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	owned := h.session(withConfirmationAttempt(h.confirmationAttemptID))

	h.sessionCQ.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
		Return(owned, nil).Once()
	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).
		Return(owned, nil).Once()
	h.merchantTx.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	_, err := h.uc.SelectCreditKind(h.ctx, SelectCreditKindInput{
		Token:                        h.publicToken,
		ExternalMerchantID:           h.merchantID,
		CreditKind:                   model.PayInInstallmentsCreditKind,
		ExpectedEvaluationGeneration: 1,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_confirmation_is_in_progress")
}

func TestSelectCreditKind_RejectsForeignMerchant(t *testing.T) {
	h := newHarness(t)
	h.expectPool(0, 1)
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	open := h.session(func(s *model.CheckoutSession) {
		s.CreditKind = ""
		s.DecisionID = nil
		s.OfferPreparationMode = model.OfferPreparationOnStart
	})
	foreignMerchant := model.ExternalMerchantID(h.sessionID.UUID())
	require.NotEqual(t, h.merchantID, foreignMerchant)

	h.sessionCQ.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
		Return(open, nil).Once()

	_, err := h.uc.SelectCreditKind(h.ctx, SelectCreditKindInput{
		Token:                        h.publicToken,
		ExternalMerchantID:           foreignMerchant,
		CreditKind:                   model.PayInInstallmentsCreditKind,
		ExpectedEvaluationGeneration: 1,
	})
	requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
}

func TestSelectCreditKind_RejectsDisabledKind(t *testing.T) {
	h := newHarness(t)
	h.expectPool(0, 1)
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()

	open := h.session(func(s *model.CheckoutSession) {
		s.CreditKind = ""
		s.DecisionID = nil
		s.OfferPreparationMode = model.OfferPreparationOnStart
	})

	h.sessionCQ.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
		Return(open, nil).Once()
	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).
		Return(open, nil).Once()
	h.merchantTx.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{
			ID:                 h.merchantPID,
			ExternalMerchantID: h.merchantID,
			Status:             model.ActiveMerchantStatus,
		}, nil).Once()
	// Harness defaults monthly to disabled via Maybe().

	_, err := h.uc.SelectCreditKind(h.ctx, SelectCreditKindInput{
		Token:                        h.publicToken,
		ExternalMerchantID:           h.merchantID,
		CreditKind:                   model.PayNextMonthCreditKind,
		ExpectedEvaluationGeneration: 1,
	})
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.credit_kind_not_enabled")
}

func TestSelectCreditKind_RequiresExpectedGeneration(t *testing.T) {
	h := newHarness(t)

	_, err := h.uc.SelectCreditKind(h.ctx, SelectCreditKindInput{
		Token:              h.publicToken,
		ExternalMerchantID: h.merchantID,
		CreditKind:         model.PayInInstallmentsCreditKind,
	})
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.expected_evaluation_generation_required")
}
