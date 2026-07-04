package checkoutsessionuc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestRequestExternalOTP_ExpiredPaymentSessionBlocksIssuance(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	session := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
	)

	expires := h.now // exactly at deadline → expired (!After)
	intent := "pi_expired"
	sessionID := "ps_expired"
	url := "https://pay.example/expired"
	contractRow := model.Contract{
		ID:                        h.contractID,
		Status:                    model.PaymentPendingContractStatus,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &url,
		FinancialPaymentExpiresAt: &expires,
	}
	attempt := model.PaymentAttempt{
		ID:                        h.attemptID,
		ContractID:                h.contractID,
		Purpose:                   model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		State:                     model.CreatedPaymentAttemptState,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		PaymentURL:                &url,
		FinancialPaymentExpiresAt: &expires,
	}

	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{ID: h.merchantPID, DisplayName: "Shop"}, nil).Once()
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.expectOTPContinuationDepsWithAttempt(session, contractRow, attempt)

	_, err := h.uc.RequestExternalOTP(h.ctx, RequestExternalOTPInput{
		Token:                h.publicToken,
		EvaluationGeneration: 1,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_otp_issuance_not_allowed")
	require.Empty(t, h.notificationCalls)

	row, getErr := h.otpChallenges.GetByPurposeSubject(
		h.ctx,
		model.OTPPurposeHostedCheckoutConfirm,
		session.ID.String(),
	)
	if getErr == nil {
		require.Equal(t, 0, row.SendCount)
		require.Nil(t, row.IssueID)
	}
}

func TestRequestExternalOTP_IncompletePaymentSessionStillEligible(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	session := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
	)

	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{ID: h.merchantPID, DisplayName: "Shop"}, nil).Once()
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.expectOTPContinuationDeps(session)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	got, err := h.uc.RequestExternalOTP(h.ctx, RequestExternalOTPInput{
		Token:                h.publicToken,
		EvaluationGeneration: 1,
	})
	require.NoError(t, err)
	require.True(t, got.OTPSent)
	require.Len(t, h.notificationCalls, 1)
}

func TestGetExternalConfirmationStatus_MissingSagaNeedsAttention(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// Session + saga lookup (projection) + saga lookup (OTP blockers) + OTP availability.
	h.expectPool(4, 0)

	session := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
		withConfirmationAttempt(h.confirmationAttemptID),
	)
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	got, err := h.uc.GetExternalConfirmationStatus(h.ctx, GetExternalConfirmationStatusInput{
		Token: h.publicToken,
	})
	require.NoError(t, err)
	require.Equal(t, ConfirmationStatusNeedsAttention, got.ConfirmationStatus)
	require.Equal(t, ConfirmationNextActionShowPendingReview, got.NextAction)
	require.Equal(t, OTPAvailabilityIssuanceBlocked, got.OTPAvailability)
}

func TestRequestExternalOTP_MissingSagaBlocksIssuance(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	session := h.session(
		withStatus(model.ProcessingCheckoutSessionStatus),
		withContract(h.contractID),
		withConfirmationAttempt(h.confirmationAttemptID),
	)

	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{ID: h.merchantPID, DisplayName: "Shop"}, nil).Once()
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	// ConfirmationAttemptID set + empty saga store → fail closed before Contract lock.

	_, err := h.uc.RequestExternalOTP(h.ctx, RequestExternalOTPInput{
		Token:                h.publicToken,
		EvaluationGeneration: 1,
	})
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_otp_issuance_not_allowed")
	require.Empty(t, h.notificationCalls)
}

func TestClassifyPaymentContinuation_ExactDeadlineIsExpired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	intent := "pi_1"
	sessionID := "ps_1"
	url := "https://pay.example/checkout"
	contractRow := model.Contract{
		Status:                    model.PaymentPendingContractStatus,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		FinancialPaymentLink:      &url,
		FinancialPaymentExpiresAt: &now,
	}
	attempt := model.PaymentAttempt{
		State:                     model.CreatedPaymentAttemptState,
		FinancialPaymentIntentID:  &intent,
		FinancialPaymentSessionID: &sessionID,
		PaymentURL:                &url,
		FinancialPaymentExpiresAt: &now,
	}

	require.Equal(t, paymentContinuationExpired, classifyPaymentContinuation(contractRow, attempt, now))
	require.True(t, paymentContinuationBlocksCustomerOTP(paymentContinuationExpired))
}
