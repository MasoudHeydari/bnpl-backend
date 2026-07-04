package checkoutsessionuc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestClassifyPaymentContinuation_Matrix(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		10,
		18,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	expires := now.Add(time.Hour)
	intent := "pi_1"
	sessionID := "ps_1"
	url := "https://pay.example/checkout"
	fcAttempt := "fpa_1"

	baseContract := func() model.Contract {
		return model.Contract{
			Status:                    model.PaymentPendingContractStatus,
			FinancialPaymentIntentID:  &intent,
			FinancialPaymentSessionID: &sessionID,
			FinancialPaymentLink:      &url,
			FinancialPaymentExpiresAt: &expires,
			FinancialPaymentAttemptID: &fcAttempt,
		}
	}
	baseAttempt := func() model.PaymentAttempt {
		return model.PaymentAttempt{
			State:                     model.CreatedPaymentAttemptState,
			FinancialPaymentIntentID:  &intent,
			FinancialPaymentSessionID: &sessionID,
			PaymentURL:                &url,
			FinancialPaymentExpiresAt: &expires,
			FinancialPaymentAttemptID: &fcAttempt,
		}
	}

	t.Run("usable", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, paymentContinuationUsable, classifyPaymentContinuation(baseContract(), baseAttempt(), now))
	})

	t.Run("contract_url_missing", func(t *testing.T) {
		t.Parallel()
		c := baseContract()
		c.FinancialPaymentLink = nil
		require.Equal(t, paymentContinuationIncomplete, classifyPaymentContinuation(c, baseAttempt(), now))
	})

	t.Run("attempt_url_missing", func(t *testing.T) {
		t.Parallel()
		a := baseAttempt()
		a.PaymentURL = nil
		require.Equal(t, paymentContinuationIncomplete, classifyPaymentContinuation(baseContract(), a, now))
	})

	t.Run("session_id_missing_on_contract", func(t *testing.T) {
		t.Parallel()
		c := baseContract()
		c.FinancialPaymentSessionID = nil
		require.Equal(t, paymentContinuationIncomplete, classifyPaymentContinuation(c, baseAttempt(), now))
	})

	t.Run("intent_id_missing_on_attempt", func(t *testing.T) {
		t.Parallel()
		a := baseAttempt()
		a.FinancialPaymentIntentID = nil
		require.Equal(t, paymentContinuationIncomplete, classifyPaymentContinuation(baseContract(), a, now))
	})

	t.Run("intent_ids_differ", func(t *testing.T) {
		t.Parallel()
		a := baseAttempt()
		other := "pi_other"
		a.FinancialPaymentIntentID = &other
		require.Equal(t, paymentContinuationMismatch, classifyPaymentContinuation(baseContract(), a, now))
	})

	t.Run("session_ids_differ", func(t *testing.T) {
		t.Parallel()
		a := baseAttempt()
		other := "ps_other"
		a.FinancialPaymentSessionID = &other
		require.Equal(t, paymentContinuationMismatch, classifyPaymentContinuation(baseContract(), a, now))
	})

	t.Run("urls_differ", func(t *testing.T) {
		t.Parallel()
		a := baseAttempt()
		other := "https://pay.example/other"
		a.PaymentURL = &other
		require.Equal(t, paymentContinuationMismatch, classifyPaymentContinuation(baseContract(), a, now))
	})

	t.Run("expirations_differ", func(t *testing.T) {
		t.Parallel()
		a := baseAttempt()
		other := expires.Add(time.Minute)
		a.FinancialPaymentExpiresAt = &other
		require.Equal(t, paymentContinuationMismatch, classifyPaymentContinuation(baseContract(), a, now))
	})

	t.Run("expired_session", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, paymentContinuationExpired, classifyPaymentContinuation(baseContract(), baseAttempt(), expires))
	})

	t.Run("fc_attempt_refs_conflict", func(t *testing.T) {
		t.Parallel()
		a := baseAttempt()
		other := "fpa_other"
		a.FinancialPaymentAttemptID = &other
		require.Equal(t, paymentContinuationMismatch, classifyPaymentContinuation(baseContract(), a, now))
	})

	t.Run("attempt_succeeded_ineligible_for_resume", func(t *testing.T) {
		t.Parallel()
		a := baseAttempt()
		a.State = model.SucceededPaymentAttemptState
		require.Equal(t, paymentContinuationIneligible, classifyPaymentContinuation(baseContract(), a, now))
	})

	t.Run("attempt_reconciliation_required", func(t *testing.T) {
		t.Parallel()
		a := baseAttempt()
		a.State = model.ReconciliationRequiredPaymentAttemptState
		require.Equal(t, paymentContinuationIneligible, classifyPaymentContinuation(baseContract(), a, now))
	})

	t.Run("attempt_manual_review_required", func(t *testing.T) {
		t.Parallel()
		a := baseAttempt()
		a.State = model.ManualReviewRequiredPaymentAttemptState
		require.Equal(t, paymentContinuationIneligible, classifyPaymentContinuation(baseContract(), a, now))
	})

	t.Run("contract_not_payment_pending", func(t *testing.T) {
		t.Parallel()
		c := baseContract()
		c.Status = model.ActiveContractStatus
		require.Equal(t, paymentContinuationIneligible, classifyPaymentContinuation(c, baseAttempt(), now))
	})
}

func TestPaymentContinuationBlocksCustomerOTP(t *testing.T) {
	t.Parallel()

	require.False(t, paymentContinuationBlocksCustomerOTP(paymentContinuationUsable))
	require.False(t, paymentContinuationBlocksCustomerOTP(paymentContinuationIncomplete))
	require.True(t, paymentContinuationBlocksCustomerOTP(paymentContinuationExpired))
	require.True(t, paymentContinuationBlocksCustomerOTP(paymentContinuationMismatch))
	require.True(t, paymentContinuationBlocksCustomerOTP(paymentContinuationIneligible))
}

func TestGetExternalConfirmationStatus_ReconfirmAfterGenerationBump(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	session := h.session(withoutDecision(), withGeneration(2))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, ConfirmationStatusReconfirmRequired, got.ConfirmationStatus)
	require.Equal(t, ConfirmationNextActionReconfirm, got.NextAction)
}

func TestGetExternalConfirmationStatus_FirstTimeOnStartWithoutDecision(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	session := h.session(withoutDecision(), withGeneration(1))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, ConfirmationStatusNotStarted, got.ConfirmationStatus)
	require.Equal(t, ConfirmationNextActionStartConfirm, got.NextAction)
}

func TestGetExternalConfirmationStatus_SucceededAttemptWhileProcessingIsNotCompleted(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(4, 1)

	session := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	intent := "pi_1"
	sessionID := "ps_1"
	url := "https://pay.example/checkout"
	expires := h.now.Add(time.Hour)
	contract := model.Contract{
		ID:                        h.contractID,
		DecisionID:                h.decisionID,
		Status:                    model.PaymentPendingContractStatus,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &url,
		FinancialPaymentExpiresAt: &expires,
	}
	h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(contract, nil).Twice()
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		ID:            h.decisionID,
		PolicyID:      "checkout_contract_approval",
		PolicyVersion: "v1",
	}, nil).Twice()

	attempt := model.PaymentAttempt{
		ID:                        h.attemptID,
		ContractID:                h.contractID,
		State:                     model.SucceededPaymentAttemptState,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		PaymentURL:                &url,
		FinancialPaymentExpiresAt: &expires,
	}
	h.attemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		h.ctx, h.contractID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(attempt, nil).Twice()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, got.Status)
	require.Equal(t, ConfirmationStatusPaymentProcessing, got.ConfirmationStatus)
	require.Equal(t, ConfirmationNextActionWait, got.NextAction)
	require.NotEqual(t, ConfirmationStatusCompleted, got.ConfirmationStatus)
	require.NotEqual(t, ConfirmationNextActionShowSuccess, got.NextAction)
}

func TestGetExternalConfirmationStatus_ActiveContractWhileProcessingIsNotCompleted(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(3, 0)

	session := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	contract := model.Contract{
		ID:         h.contractID,
		DecisionID: h.decisionID,
		Status:     model.ActiveContractStatus,
	}
	h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(contract, nil).Twice()
	// Trusted / merchant-funding policy → payment not required.
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		ID:            h.decisionID,
		PolicyID:      "checkout_contract_approval",
		PolicyVersion: "v2",
	}, nil).Twice()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, ConfirmationStatusProcessing, got.ConfirmationStatus)
	require.Equal(t, ConfirmationNextActionWait, got.NextAction)
	require.NotEqual(t, ConfirmationStatusCompleted, got.ConfirmationStatus)
}

func TestGetExternalConfirmationStatus_PaymentActionRequiredWhenSessionConsistent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(4, 1)

	session := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	intent := "pi_1"
	sessionID := "ps_1"
	url := "https://pay.example/checkout"
	expires := h.now.Add(time.Hour)
	contract := model.Contract{
		ID:                        h.contractID,
		DecisionID:                h.decisionID,
		Status:                    model.PaymentPendingContractStatus,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &url,
		FinancialPaymentExpiresAt: &expires,
	}
	h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(contract, nil).Twice()
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		ID:            h.decisionID,
		PolicyID:      "checkout_contract_approval",
		PolicyVersion: "v1",
	}, nil).Twice()

	attempt := model.PaymentAttempt{
		ID:                        h.attemptID,
		ContractID:                h.contractID,
		State:                     model.SubmittedPaymentAttemptState,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		PaymentURL:                &url,
		FinancialPaymentExpiresAt: &expires,
	}
	h.attemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		h.ctx, h.contractID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(attempt, nil).Twice()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, ConfirmationStatusPaymentActionRequired, got.ConfirmationStatus)
	require.Equal(t, ConfirmationNextActionResumePayment, got.NextAction)
}

func TestGetExternalConfirmationStatus_MismatchedSessionNeedsAttention(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(4, 1)

	session := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	intentA := "pi_a"
	intentB := "pi_b"
	sessionID := "ps_1"
	url := "https://pay.example/checkout"
	expires := h.now.Add(time.Hour)
	contract := model.Contract{
		ID:                        h.contractID,
		DecisionID:                h.decisionID,
		Status:                    model.PaymentPendingContractStatus,
		FinancialPaymentIntentID:  &intentA,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &url,
		FinancialPaymentExpiresAt: &expires,
	}
	h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(contract, nil).Twice()
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		ID:            h.decisionID,
		PolicyID:      "checkout_contract_approval",
		PolicyVersion: "v1",
	}, nil).Twice()

	attempt := model.PaymentAttempt{
		ID:                        h.attemptID,
		ContractID:                h.contractID,
		State:                     model.CreatedPaymentAttemptState,
		FinancialPaymentIntentID:  &intentB,
		FinancialPaymentSessionID: &sessionID,
		PaymentURL:                &url,
		FinancialPaymentExpiresAt: &expires,
	}
	h.attemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		h.ctx, h.contractID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(attempt, nil).Twice()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, ConfirmationStatusNeedsAttention, got.ConfirmationStatus)
	require.Equal(t, ConfirmationNextActionShowPendingReview, got.NextAction)
}

func TestGetExternalConfirmationStatus_ExpiredPaymentSessionIsNotResume(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(4, 1)

	session := h.session(withStatus(model.ProcessingCheckoutSessionStatus), withContract(h.contractID))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	intent := "pi_1"
	sessionID := "ps_1"
	url := "https://pay.example/checkout"
	expires := h.now.Add(-time.Minute)
	contract := model.Contract{
		ID:                        h.contractID,
		DecisionID:                h.decisionID,
		Status:                    model.PaymentPendingContractStatus,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &url,
		FinancialPaymentExpiresAt: &expires,
	}
	h.contractCQ.EXPECT().GetByID(h.ctx, h.contractID).Return(contract, nil).Twice()
	h.decisionCQ.EXPECT().GetByID(h.ctx, h.decisionID).Return(model.Decision{
		ID:            h.decisionID,
		PolicyID:      "checkout_contract_approval",
		PolicyVersion: "v1",
	}, nil).Twice()

	attempt := model.PaymentAttempt{
		ID:                        h.attemptID,
		ContractID:                h.contractID,
		State:                     model.SubmittedPaymentAttemptState,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		PaymentURL:                &url,
		FinancialPaymentExpiresAt: &expires,
	}
	h.attemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		h.ctx, h.contractID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(attempt, nil).Twice()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, ConfirmationStatusPaymentProcessing, got.ConfirmationStatus)
	require.Equal(t, ConfirmationNextActionWait, got.NextAction)
}

func TestGetExternalConfirmationStatus_CanceledAndUnavailable(t *testing.T) {
	t.Parallel()

	t.Run("canceled", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.expectPool(1, 0)
		session := h.session(withStatus(model.CanceledCheckoutSessionStatus))
		h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

		got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
		require.NoError(t, err)
		require.Equal(t, ConfirmationStatusCanceled, got.ConfirmationStatus)
		require.Equal(t, ConfirmationNextActionShowCanceled, got.NextAction)
	})

	t.Run("unavailable", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t)
		h.expectPool(1, 0)
		session := h.session(withStatus(model.UnavailableCheckoutSessionStatus))
		h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

		got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{Token: h.publicToken})
		require.NoError(t, err)
		require.Equal(t, ConfirmationStatusUnavailable, got.ConfirmationStatus)
		require.Equal(t, ConfirmationNextActionShowUnavailable, got.NextAction)
	})
}
