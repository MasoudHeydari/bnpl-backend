package checkoutsessionuc

import (
	"context"
	"errors"
	"testing"
	"time"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/checkoutotp"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutotppolicy "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutotp"
)

const testOTPCode = "123456"

// withOTPVerifier stores the keyed verifier of code on the session, bound to
// the session's current evaluation generation under a fresh otp_issue_id.
func withOTPVerifier(t *testing.T, token model.CheckoutSessionToken, code string, mods ...func(*model.CheckoutSession)) func(*model.CheckoutSession) {
	t.Helper()

	return func(s *model.CheckoutSession) {
		verifier, err := newTestOTPHasher(t).Hash(checkoutotp.DomainSeparatedMaterial(
			token.String(),
			s.EvaluationGeneration,
			code,
		))
		require.NoError(t, err)

		issueID := model.OTPIssueID("oi_0123456789abcdef0123456789abcdef")
		value := verifier.String()
		gen := s.EvaluationGeneration
		s.OTPIssueID = &issueID
		s.OTPVerifier = &value
		s.OTPEvaluationGeneration = &gen
		expiresAt := s.UpdatedAt.Add(time.Hour * 24)
		s.OTPExpiresAt = &expiresAt

		for _, mod := range mods {
			mod(s)
		}
	}
}

func withOTPExpiresAt(at time.Time) func(*model.CheckoutSession) {
	return func(s *model.CheckoutSession) { s.OTPExpiresAt = &at }
}

func withOTPAttempts(count int) func(*model.CheckoutSession) {
	return func(s *model.CheckoutSession) { s.OTPAttemptCount = count }
}

func withOTPNotificationAcceptedAt(at time.Time) func(*model.CheckoutSession) {
	return func(s *model.CheckoutSession) { s.OTPNotificationAcceptedAt = &at }
}

func withOTPVerifiedAt(at time.Time) func(*model.CheckoutSession) {
	return func(s *model.CheckoutSession) {
		s.OTPVerifiedAt = &at
		gen := s.EvaluationGeneration
		s.OTPEvaluationGeneration = &gen
	}
}

func withOrigin(origin model.CheckoutSessionOrigin) func(*model.CheckoutSession) {
	return func(s *model.CheckoutSession) { s.Origin = origin }
}

func (h *harness) approvedDecision() {
	h.setApprovedDecisionCreateOutput()
}

// expectExternalBind expects the bind transaction for an unbound session.
func (h *harness) expectExternalBind(locked model.CheckoutSession, bound model.CheckoutSession) {
	h.t.Helper()

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(locked, nil).Once()
	h.customerTx.EXPECT().
		EnsureExists(h.ctx, h.profileID, model.IRJurisdiction, model.ActiveCustomerStatus, h.now, h.now).
		Return(h.customerProfile(), false, nil).Once()
	h.sessionTx.EXPECT().BindCustomer(h.ctx, locked.ID, h.customerID, h.now).Return(bound, nil).Once()
}

func TestGetExternal_ProjectsOnlyHostedPageFields(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	session := h.session(withoutDecision())
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{ID: h.merchantPID, DisplayName: "Shop"}, nil).Once()

	got, err := h.uc.GetExternal(h.ctx, GetExternalInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, GetExternalOutput{
		Token:                h.publicToken,
		Status:               model.OpenCheckoutSessionStatus,
		MerchantDisplayName:  "Shop",
		AmountMinor:          150000000,
		Currency:             model.IRRCurrency,
		EvaluationGeneration: 1,
		Verification: ExternalVerification{
			Type:        VerificationTypeSMSOTP,
			Destination: "0912***4567",
		},
		ExpiresAt: session.ExpiresAt,
	}, got)
	require.Nil(t, got.Offer)
}

func TestGetExternal_ProjectsPreparedOfferWithoutSideEffects(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(2, 0)

	prepared := h.session()
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(prepared, nil).Once()
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{ID: h.merchantPID, DisplayName: "Shop"}, nil).Once()
	h.expectProjectApprovedOffer(prepared)

	identityCallsBefore := len(h.identityCalls)
	decisionCreatesBefore := len(h.decisions.calls)

	got, err := h.uc.GetExternal(h.ctx, GetExternalInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, 1, got.EvaluationGeneration)
	require.NotNil(t, got.Offer)
	require.Equal(t, ApprovedOfferOutcome, got.Offer.Outcome)
	require.Zero(t, got.Offer.AvailableCreditMinor)
	require.Len(t, h.identityCalls, identityCallsBefore)
	require.Len(t, h.decisions.calls, decisionCreatesBefore)
	require.Empty(t, h.notificationCalls)
}

func TestGetExternal_ProjectsElapsedOpenSessionAsExpired(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	session := h.session(withoutDecision(), withExpiresAt(h.now.Add(-time.Second)))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.merchantCQ.EXPECT().GetByID(h.ctx, h.merchantPID).Return(model.MerchantProfile{}, nil).Once()

	got, err := h.uc.GetExternal(h.ctx, GetExternalInput{Token: h.publicToken})
	require.NoError(t, err)
	require.Equal(t, model.ExpiredCheckoutSessionStatus, got.Status)
}

func TestGetExternal_InternalOriginIsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	h.sessionCQ.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
		Return(h.session(withOrigin(model.InternalCheckoutSessionOrigin)), nil).Once()

	_, err := h.uc.GetExternal(h.ctx, GetExternalInput{Token: h.publicToken})
	requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
}

func TestGetExternal_RejectsMalformedToken(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.uc.GetExternal(h.ctx, GetExternalInput{Token: "not-a-token"})
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.checkout_session_id_invalid")
}

func TestStartExternal_ApprovedOfferIssuesAndSendsOTPOutsideTransactions(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// begin OTP, project offer, merchant display, issue OTP, mark sent.
	h.expectPool(2, 3)

	prepared := h.session()
	h.expectProjectApprovedOffer(prepared)

	var verifier string

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
			verifier = got
			issued.OTPIssueID = &issueID
			issued.OTPVerifier = &got
			issued.OTPEvaluationGeneration = &evaluationGeneration
			issued.OTPExpiresAt = &expiresAt
			issued.OTPSendCount = 1

			return issued, nil
		}).Once()

	h.merchantCQ.EXPECT().GetByID(h.ctx, issued.MerchantProfileID).Return(model.MerchantProfile{
		ID:          issued.MerchantProfileID,
		DisplayName: "Digikala",
	}, nil).Once()

	h.sessionTx.EXPECT().
		MarkOTPNotificationAccepted(h.ctx, h.sessionID, testifymock.Anything, h.now, "req-1", h.now).
		RunAndReturn(func(
			_ context.Context,
			_ model.CheckoutSessionID,
			_ model.OTPIssueID,
			sentAt time.Time,
			_ string,
			_ time.Time,
		) (model.CheckoutSession, error) {
			sent := issued
			sent.OTPNotificationAcceptedAt = &sentAt

			return sent, nil
		}).Once()

	got, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	require.NoError(t, err)
	require.True(t, got.OTPSent)
	require.Equal(t, 1, got.EvaluationGeneration)
	require.NotNil(t, got.OTPExpiresAt)
	require.Empty(t, h.identityCalls)

	// Exactly one Notification submit; the submitted code matches the stored
	// verifier and the verifier is a keyed hash, never the plaintext code.
	require.Len(t, h.notificationCalls, 1)
	notif := h.notificationCalls[0]
	require.Equal(t, "+989121234567", notif.Recipient)
	require.Equal(t, testOTPTemplateID, notif.TemplateID)
	require.Equal(t, "Digikala", notif.Variables[otpNotificationMerchantNameParam])
	require.NotNil(t, issued.OTPExpiresAt)
	require.Equal(t, issued.OTPExpiresAt.UTC(), notif.ExpiresAt.UTC())

	code := notif.Variables[otpNotificationCodeParam]
	require.True(t, checkoutotp.IsWellFormed(code))
	require.NotContains(t, verifier, code)

	matches, err := newTestOTPHasher(t).Verify(
		checkoutotp.DomainSeparatedMaterial(h.publicToken.String(), 1, code),
		verifier,
	)
	require.NoError(t, err)
	require.True(t, matches)
}

func TestStartExternal_ActiveOTPReplaysWithoutResending(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// begin, project offer, merchant display, issue OTP guard (idempotent).
	h.expectPool(2, 2)

	active := h.session(withOTPVerifier(
		t,
		h.publicToken,
		testOTPCode,
		withOTPExpiresAt(h.now.Add(time.Minute)),
		withOTPNotificationAcceptedAt(h.now.Add(-time.Minute)),
	))

	h.expectProjectApprovedOffer(active)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(active, nil).Times(2)
	h.merchantCQ.EXPECT().GetByID(h.ctx, active.MerchantProfileID).Return(model.MerchantProfile{
		ID:          active.MerchantProfileID,
		DisplayName: "Shop",
	}, nil).Once()

	got, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	require.NoError(t, err)
	require.False(t, got.OTPSent)
	require.NotNil(t, got.OTPExpiresAt)
	require.Empty(t, h.notificationCalls)
	require.Empty(t, h.identityCalls)
}

func TestStartExternal_DeniedOfferNeverIssuesOTP(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// begin + project offer only.
	h.expectPool(1, 1)

	prepared := h.session()
	h.expectProjectDeniedOffer(prepared)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(prepared, nil).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_cannot_continue_in_its_current_state")
	require.Empty(t, h.notificationCalls)
	require.Empty(t, h.identityCalls)
}

func TestStartExternal_UnpreparedOfferIsNotReady(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).
		Return(h.session(withCustomer(nil), withoutDecision()), nil).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_is_not_ready")
	require.Empty(t, h.notificationCalls)
	require.Empty(t, h.identityCalls)
}

func TestStartExternal_StaleGenerationDoesNotConsumeOTPBudget(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	current := h.session(withGeneration(2))
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(current, nil).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_reconfirm_required")
	require.Empty(t, h.notificationCalls)
	require.Empty(t, h.identityCalls)
}

func TestStartExternal_SMSFailureLeavesOTPUnsent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// begin, project, merchant display, issue OTP; MarkOTPNotificationAccepted never runs.
	h.expectPool(2, 2)

	sendErr := errors.New("sms provider down")
	h.notificationErr = sendErr

	prepared := h.session()
	h.expectProjectApprovedOffer(prepared)
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(prepared, nil).Times(2)
	h.expectOTPRateLimits()
	h.sessionTx.EXPECT().
		IssueOTP(h.ctx, h.sessionID, testifymock.Anything, testifymock.Anything, 1, h.now.Add(2*time.Minute), h.now).
		Return(prepared, nil).Once()
	h.merchantCQ.EXPECT().GetByID(h.ctx, prepared.MerchantProfileID).Return(model.MerchantProfile{
		ID:          prepared.MerchantProfileID,
		DisplayName: "Digikala",
	}, nil).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	require.ErrorIs(t, err, sendErr)
}

func TestStartExternal_InternalOriginIsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).
		Return(h.session(withOrigin(model.InternalCheckoutSessionOrigin)), nil).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
	require.Empty(t, h.identityCalls)
}

func TestStartExternal_ElapsedSessionIsExpiredBeforeAnyNetworkCall(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	elapsed := h.session(withoutDecision(), withExpiresAt(h.now.Add(-time.Second)))
	expired := h.session(withStatus(model.ExpiredCheckoutSessionStatus))

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(elapsed, nil).Once()
	h.sessionTx.EXPECT().
		UpdateStatus(h.ctx, h.sessionID, model.ExpiredCheckoutSessionStatus, h.now).
		Return(expired, nil).Once()
	h.expectLifecycleEvent(expired, checkoutlifecycle.EventTypeExpired)

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
	require.Empty(t, h.identityCalls)
	require.Empty(t, h.notificationCalls)
}

func (h *harness) confirmExternalInput(otp string, generation int) ConfirmExternalInput {
	return ConfirmExternalInput{Token: h.publicToken, OTP: otp, EvaluationGeneration: generation}
}

func TestConfirmExternal_ValidOTPMarksVerifiedAndRunsConfirmCore(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// verify OTP, then the Confirm claim transaction.
	h.expectPool(0, 2)

	pending := h.session(withoutDecision(), withOTPVerifier(t, h.publicToken, testOTPCode))
	verified := h.session(withoutDecision(), withOTPVerifiedAt(h.now))

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.sessionTx.EXPECT().MarkOTPVerified(h.ctx, h.sessionID, h.now, h.now).Return(verified, nil).Once()
	h.customerTx.EXPECT().LockByID(h.ctx, h.customerID).Return(h.customerProfile(), nil).Once()

	// Confirm core stops at the missing Decision, after locking the customer the
	// OTP step resolved (profile id from the session binding, not the request).
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(verified, nil).Once()
	h.expectCustomerLock()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_is_not_ready")
	require.Empty(t, h.contracts.calls)
}

func TestConfirmExternal_VerifiedSessionSkipsOTPChallenge(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 2)

	verified := h.session(withoutDecision(), withOTPVerifiedAt(h.now.Add(-time.Minute)))

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(verified, nil).Once()
	h.customerTx.EXPECT().LockByID(h.ctx, h.customerID).Return(h.customerProfile(), nil).Once()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(verified, nil).Once()
	h.expectCustomerLock()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_is_not_ready")
}

func TestConfirmExternal_WrongOTPRecordsFailureAndRejects(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	pending := h.session(withOTPVerifier(t, h.publicToken, testOTPCode))
	failed := h.session(withOTPVerifier(t, h.publicToken, testOTPCode), withOTPAttempts(1))

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.sessionTx.EXPECT().RecordOTPFailure(h.ctx, h.sessionID, h.now).Return(failed, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput("654321", 1))
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.checkout_otp_invalid")
}

func TestConfirmExternal_LastWrongAttemptReportsLocked(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	maxAttempts := checkoutotppolicy.V1().Terms().MaxVerificationAttempts
	pending := h.session(withOTPVerifier(t, h.publicToken, testOTPCode), withOTPAttempts(maxAttempts-1))
	failed := h.session(withOTPVerifier(t, h.publicToken, testOTPCode), withOTPAttempts(maxAttempts))

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.sessionTx.EXPECT().RecordOTPFailure(h.ctx, h.sessionID, h.now).Return(failed, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput("654321", 1))
	requireCerr(t, err, cerr.CodeForbidden, "error.checkout_otp_locked")
}

func TestConfirmExternal_ExhaustedAttemptsRejectEvenTheCorrectCode(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	maxAttempts := checkoutotppolicy.V1().Terms().MaxVerificationAttempts
	locked := h.session(withOTPVerifier(t, h.publicToken, testOTPCode), withOTPAttempts(maxAttempts))

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(locked, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeForbidden, "error.checkout_otp_locked")
}

func TestConfirmExternal_ExpiredOTPIsRejectedWithoutCountingAnAttempt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	pending := h.session(withOTPVerifier(t, h.publicToken, testOTPCode, withOTPExpiresAt(h.now)))

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_otp_expired")
}

func TestConfirmExternal_StaleEvaluationGenerationRejectsBeforeConsumingOTPAttempt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	// Session/offer are at generation 2; client still presents generation 1.
	pending := h.session(withGeneration(2), withOTPVerifier(t, h.publicToken, testOTPCode))

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	// RecordOTPFailure must not be expected: stale generation is detected first.

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_reconfirm_required")
}

func TestConfirmExternal_NoIssuedOTPIsNotRequested(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(h.session(), nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_otp_not_requested")
}

func TestConfirmExternal_UnstartedSessionIsNotReady(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).
		Return(h.session(withCustomer(nil)), nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_is_not_ready")
}

func TestConfirmExternal_InternalOriginIsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).
		Return(h.session(withOrigin(model.InternalCheckoutSessionOrigin)), nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
}

func TestConfirmExternal_MalformedInputIsRejectedBeforeAnyRead(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput("", 1))
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.checkout_otp_required")

	_, err = h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput("12345a", 1))
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.checkout_otp_invalid")

	_, err = h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 0))
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.evaluation_generation_required")
}

func TestConfirmExternal_MissingSnapshottedAuthScopeFailsClosed(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	session := h.session()
	session.AuthTenantID = nil
	session.AuthContextID = nil

	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeInternalError, cerr.DefaultMessageKey(cerr.CodeInternalError))
}
