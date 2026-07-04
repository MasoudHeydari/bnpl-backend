package checkoutsessionuc

import (
	"context"
	"errors"
	"testing"
	"time"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestStartExternal_InFlightIssuanceBlocksSecondIssue(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// begin, project offer, merchant display, issueOTP (idempotent).
	h.expectPool(2, 2)

	// OTP1 issued but not yet notification-accepted: otp_notification_accepted_at is nil.
	inFlight := h.session(withOTPVerifier(
		t,
		h.publicToken,
		testOTPCode,
		withOTPExpiresAt(h.now.Add(time.Minute)),
	))
	require.Nil(t, inFlight.OTPNotificationAcceptedAt)
	require.NotNil(t, inFlight.OTPIssueID)

	h.expectProjectApprovedOffer(inFlight)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(inFlight, nil).Times(2)
	h.merchantCQ.EXPECT().GetByID(h.ctx, inFlight.MerchantProfileID).Return(model.MerchantProfile{
		ID:          inFlight.MerchantProfileID,
		DisplayName: "Shop",
	}, nil).Once()

	got, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	require.NoError(t, err)
	require.False(t, got.OTPSent)
	require.Empty(t, h.notificationCalls)
	require.NotNil(t, got.OTPExpiresAt)
}

func TestStartExternal_SMSIdempotencyKeyUsesIssueID(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(2, 3)

	prepared := h.session()
	h.expectProjectApprovedOffer(prepared)

	var capturedIssueID model.OTPIssueID
	issued := h.session()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(prepared, nil).Times(2)
	h.expectOTPRateLimits()
	h.sessionTx.EXPECT().
		IssueOTP(h.ctx, h.sessionID, testifymock.Anything, testifymock.Anything, 1, h.now.Add(2*time.Minute), h.now).
		RunAndReturn(func(
			_ context.Context,
			_ model.CheckoutSessionID,
			issueID model.OTPIssueID,
			got string,
			evaluationGeneration int,
			expiresAt time.Time,
			_ time.Time,
		) (model.CheckoutSession, error) {
			capturedIssueID = issueID
			issued.OTPIssueID = &issueID
			issued.OTPVerifier = &got
			issued.OTPEvaluationGeneration = &evaluationGeneration
			issued.OTPExpiresAt = &expiresAt
			issued.OTPSendCount = 1

			return issued, nil
		}).Once()

	h.merchantCQ.EXPECT().GetByID(h.ctx, issued.MerchantProfileID).Return(model.MerchantProfile{
		ID:          issued.MerchantProfileID,
		DisplayName: "Shop",
	}, nil).Once()
	h.sessionTx.EXPECT().
		MarkOTPNotificationAccepted(h.ctx, h.sessionID, testifymock.Anything, h.now, "req-1", h.now).
		Return(issued, nil).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	require.NoError(t, err)
	require.Len(t, h.notificationCalls, 1)
	require.Equal(
		t,
		otpNotificationIdempotencyPrefix+h.publicToken.String()+":"+capturedIssueID.String(),
		h.notificationCalls[0].IdempotencyKey,
	)
}

func TestStartExternal_RejectedSMSClearsMatchingIssueOnly(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(2, 3)
	h.notificationErr = notification.Rejected(errors.New("provider rejected"))

	prepared := h.session()
	h.expectProjectApprovedOffer(prepared)

	var issueID model.OTPIssueID
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(prepared, nil).Times(2)
	h.expectOTPRateLimits()
	h.sessionTx.EXPECT().
		IssueOTP(h.ctx, h.sessionID, testifymock.Anything, testifymock.Anything, 1, h.now.Add(2*time.Minute), h.now).
		RunAndReturn(func(
			_ context.Context,
			_ model.CheckoutSessionID,
			got model.OTPIssueID,
			_ string,
			_ int,
			_ time.Time,
			_ time.Time,
		) (model.CheckoutSession, error) {
			issueID = got
			out := prepared
			out.OTPIssueID = &got
			out.OTPSendCount = 1

			return out, nil
		}).Once()
	h.merchantCQ.EXPECT().GetByID(h.ctx, prepared.MerchantProfileID).Return(model.MerchantProfile{
		ID:          prepared.MerchantProfileID,
		DisplayName: "Shop",
	}, nil).Once()
	h.sessionTx.EXPECT().
		ClearOTPIssue(h.ctx, h.sessionID, testifymock.Anything, h.now).
		RunAndReturn(func(
			_ context.Context,
			_ model.CheckoutSessionID,
			got model.OTPIssueID,
			_ time.Time,
		) (model.CheckoutSession, error) {
			require.Equal(t, issueID, got)

			return prepared, nil
		}).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	require.Error(t, err)
	require.True(t, notification.IsRejected(err))
}

func TestOtpActiveAt_TreatsUnsentIssueAsReservation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	issueID := model.OTPIssueID("oi_0123456789abcdef0123456789abcdef")
	expires := now.Add(time.Minute)
	gen := 1
	verifier := "hash"

	session := model.CheckoutSession{
		EvaluationGeneration:    gen,
		OTPIssueID:              &issueID,
		OTPVerifier:             &verifier,
		OTPExpiresAt:            &expires,
		OTPEvaluationGeneration: &gen,
		// OTPNotificationAcceptedAt deliberately nil: delivery still in flight.
	}

	require.True(t, otpActiveAt(session, now))
	require.False(t, otpActiveAt(session, expires))
}
