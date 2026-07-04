package checkoutsessionuc

import (
	"errors"
	"strconv"
	"testing"
	"time"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/otpcode"
	otppolicy "github.com/parstis/bnpl-backend/pkg/core/policy/otp"
)

const testOTPCode = "123456"

func (h *harness) seedOTPChallenge(
	session model.CheckoutSession,
	code string,
	mods ...func(*model.OTPChallenge),
) model.OTPChallenge {
	h.t.Helper()

	verifier, err := h.uc.otpHasher.Hash(otpcode.DomainSeparatedMaterial(
		string(model.OTPPurposeHostedCheckoutConfirm),
		session.PublicToken.String(),
		strconv.Itoa(session.EvaluationGeneration),
		code,
	))
	require.NoError(h.t, err)

	issueID := model.OTPIssueID("oi_0123456789abcdef0123456789abcdef")
	value := verifier.String()
	expiresAt := session.UpdatedAt.Add(24 * time.Hour)
	now := h.now
	challenge := model.OTPChallenge{
		ID:             model.OTPChallengeID(session.ID.UUID()),
		Purpose:        model.OTPPurposeHostedCheckoutConfirm,
		SubjectKey:     session.ID.String(),
		ContextVersion: strconv.Itoa(session.EvaluationGeneration),
		IssueID:        &issueID,
		Verifier:       &value,
		ExpiresAt:      &expiresAt,
		NextIssueAt:    &expiresAt,
		SendCount:      1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	for _, mod := range mods {
		mod(&challenge)
	}
	h.otpChallenges.byKey[challengeKey(challenge.Purpose, challenge.SubjectKey)] = challenge
	return challenge
}

func withOTPExpiresAt(at time.Time) func(*model.OTPChallenge) {
	return func(c *model.OTPChallenge) {
		c.ExpiresAt = &at
		c.NextIssueAt = &at
	}
}

func withOTPAttempts(count int) func(*model.OTPChallenge) {
	return func(c *model.OTPChallenge) { c.AttemptCount = count }
}

func withOTPNotificationAcceptedAt(at time.Time) func(*model.OTPChallenge) {
	return func(c *model.OTPChallenge) { c.NotificationAcceptedAt = &at }
}

func withOTPVerifiedAt(at time.Time) func(*model.OTPChallenge) {
	return func(c *model.OTPChallenge) {
		c.VerifiedAt = &at
		c.IssueID = nil
		c.Verifier = nil
		c.ExpiresAt = nil
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
		EnsureExists(
			h.ctx,
			h.profileID,
			model.IRJurisdiction,
			model.ActiveCustomerStatus,
			h.now,
			h.now,
		).
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

func TestStartExternal_ReturnsPreparedOfferWithoutSendingOTP(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(2, 0)
	h.expectStartExternalPrepareLoad()

	prepared := h.session()
	h.expectProjectApprovedOffer(prepared)

	got, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	require.NoError(t, err)
	require.Equal(t, 1, got.EvaluationGeneration)
	require.Equal(t, ApprovedOfferOutcome, got.Offer.Outcome)
	require.Empty(t, h.notificationCalls)
	require.Empty(t, h.identityCalls)
}

func TestStartExternal_DeniedOfferNeverIssuesOTP(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(2, 0)
	h.expectStartExternalPrepareLoad()

	prepared := h.session()
	h.expectProjectDeniedOffer(prepared)

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_cannot_continue_in_its_current_state")
	require.Empty(t, h.notificationCalls)
	require.Empty(t, h.identityCalls)
}

func TestStartExternal_LinkedContractCannotStart(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	linked := h.session(withContract(h.contractID))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(linked, nil).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_cannot_continue_in_its_current_state")
	require.Empty(t, h.notificationCalls)
	require.Empty(t, h.identityCalls)
}

func TestStartExternal_StaleGenerationDoesNotPrepareAgain(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	current := h.session(withGeneration(2))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(current, nil).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_reconfirm_required")
	require.Empty(t, h.notificationCalls)
	require.Empty(t, h.identityCalls)
}

func TestStartExternal_InternalOriginIsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	h.sessionCQ.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
		Return(h.session(withOrigin(model.InternalCheckoutSessionOrigin)), nil).Once()

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	requireCerr(t, err, cerr.CodeResourceNotFound, "error.checkout_session_not_found")
	require.Empty(t, h.identityCalls)
}

func TestStartExternal_ElapsedSessionIsExpiredBeforeAnyNetworkCall(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 0)

	elapsed := h.session(withExpiresAt(h.now.Add(-time.Second)))
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(elapsed, nil).Once()
	h.expectProjectApprovedOffer(elapsed)

	_, err := h.uc.StartExternal(h.ctx, h.startExternalInput(1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_session_expired")
	require.Empty(t, h.identityCalls)
	require.Empty(t, h.notificationCalls)
}

func (h *harness) confirmExternalInput(otp string, generation int) ConfirmExternalInput {
	return ConfirmExternalInput{Token: h.publicToken, OTP: otp, EvaluationGeneration: generation}
}

func TestConfirmExternal_UnstartedSessionIsNotReady(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	h.sessionTx.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).Return(h.session(withCustomer(nil)), nil).Once()
	h.sessionTx.EXPECT().
		LockByPublicToken(h.ctx, h.publicToken).Return(h.session(withCustomer(nil)), nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_is_not_ready")
}

func TestConfirmExternal_InternalOriginIsNotFound(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	h.sessionTx.EXPECT().
		GetByPublicToken(h.ctx, h.publicToken).
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

func TestRequestExternalOTP_IssuesAndSendsOTPOutsideTransactions(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	session := h.session()
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{ID: h.merchantPID, DisplayName: "Digikala"}, nil).Once()
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	got, err := h.uc.RequestExternalOTP(h.ctx, RequestExternalOTPInput{
		Token:                h.publicToken,
		EvaluationGeneration: 1,
	})
	require.NoError(t, err)
	require.True(t, got.OTPSent)
	require.Equal(t, OTPAvailabilityAwaitingEntry, got.OTPAvailability)
	require.NotNil(t, got.OTPExpiresAt)
	require.Len(t, h.notificationCalls, 1)
	notif := h.notificationCalls[0]
	require.Equal(t, "Digikala", notif.Variables[otpNotificationMerchantNameParam])
	require.True(t, otpcode.IsWellFormed(notif.Variables["code"]))
}

func TestRequestExternalOTP_ActiveOTPReplaysWithoutResending(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)

	active := h.session()
	h.seedOTPChallenge(active, testOTPCode, withOTPExpiresAt(h.now.Add(time.Minute)))

	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(active, nil).Once()
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{ID: h.merchantPID, DisplayName: "Shop"}, nil).Once()
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(active, nil).Once()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(active, nil).Once()

	got, err := h.uc.RequestExternalOTP(h.ctx, RequestExternalOTPInput{
		Token:                h.publicToken,
		EvaluationGeneration: 1,
	})
	require.NoError(t, err)
	require.False(t, got.OTPSent)
	require.Empty(t, h.notificationCalls)
}

func TestRequestExternalOTP_SMSFailureLeavesOTPUnsent(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(1, 1)
	h.notification.ExpectedCalls = nil
	h.notification.Calls = nil
	h.notification.EXPECT().
		Submit(testifymock.Anything, testifymock.Anything).
		Return(notification.SubmitOutput{}, notification.Rejected(errors.New("provider rejected"))).
		Once()

	session := h.session()
	h.sessionCQ.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.merchantCQ.EXPECT().
		GetByID(h.ctx, h.merchantPID).
		Return(model.MerchantProfile{ID: h.merchantPID, DisplayName: "Shop"}, nil).Once()
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(session, nil).Once()

	_, err := h.uc.RequestExternalOTP(h.ctx, RequestExternalOTPInput{
		Token:                h.publicToken,
		EvaluationGeneration: 1,
	})
	require.Error(t, err)
	require.True(t, notification.IsRejected(err))

	row, getErr := h.otpChallenges.GetByPurposeSubject(
		h.ctx,
		model.OTPPurposeHostedCheckoutConfirm,
		session.ID.String(),
	)
	require.NoError(t, getErr)
	require.Nil(t, row.IssueID)
	require.Nil(t, row.Verifier)
}

func TestConfirmExternal_ValidOTPMarksVerifiedAndRunsConfirmCore(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	// verify OTP, then the Confirm claim transaction.
	h.expectPool(0, 2)

	pending := h.session(withoutDecision())
	h.seedOTPChallenge(pending, testOTPCode)
	verified := h.session(withoutDecision())

	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.customerTx.EXPECT().LockByID(h.ctx, h.customerID).Return(h.customerProfile(), nil).Once()

	// Confirm core stops at the missing Decision, after locking the customer the
	// OTP step resolved (profile id from the session binding, not the request).
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(verified, nil).Once()
	h.expectCustomerLock()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_is_not_ready")
	require.Empty(t, h.contracts.calls)
}

func TestConfirmExternal_WrongOTPRecordsFailureAndRejects(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)

	pending := h.session()
	h.seedOTPChallenge(pending, testOTPCode)

	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput("000000", 1))
	requireCerr(t, err, cerr.CodeInvalidArgument, "error.otp_invalid")

	row, getErr := h.otpChallenges.GetByPurposeSubject(
		h.ctx, model.OTPPurposeHostedCheckoutConfirm, pending.ID.String(),
	)
	require.NoError(t, getErr)
	require.Equal(t, 1, row.AttemptCount)
}

func TestConfirmExternal_LastWrongAttemptReportsLocked(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	maxAttempts := otppolicy.V1().Terms().MaxVerificationAttempts
	pending := h.session()
	h.seedOTPChallenge(pending, testOTPCode, withOTPAttempts(maxAttempts-1))
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput("000000", 1))
	requireCerr(t, err, cerr.CodeForbidden, "error.otp_locked")
}

func TestConfirmExternal_ExhaustedAttemptsRejectEvenTheCorrectCode(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	maxAttempts := otppolicy.V1().Terms().MaxVerificationAttempts
	locked := h.session()
	h.seedOTPChallenge(locked, testOTPCode, withOTPAttempts(maxAttempts))
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(locked, nil).Once()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(locked, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeForbidden, "error.otp_locked")
}

func TestConfirmExternal_ExpiredOTPIsRejectedWithoutCountingAnAttempt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	pending := h.session()
	h.seedOTPChallenge(pending, testOTPCode, withOTPExpiresAt(h.now))
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.otp_expired")
}

func TestConfirmExternal_StaleEvaluationGenerationRejectsBeforeConsumingOTPAttempt(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	pending := h.session(withGeneration(2))
	h.seedOTPChallenge(pending, testOTPCode)
	// Generation mismatch is detected on peek — no FOR UPDATE / consume.
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.checkout_offer_reconfirm_required")
}

func TestConfirmExternal_NoIssuedOTPIsNotRequested(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	pending := h.session()
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput(testOTPCode, 1))
	requireCerr(t, err, cerr.CodeConflict, "error.otp_not_requested")
}

func TestConfirmExternal_ConsumedOTPRejectsArbitraryCode(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.expectPool(0, 1)
	pending := h.session()
	h.seedOTPChallenge(pending, testOTPCode, withOTPVerifiedAt(h.now))
	h.sessionTx.EXPECT().GetByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()
	h.sessionTx.EXPECT().LockByPublicToken(h.ctx, h.publicToken).Return(pending, nil).Once()

	_, err := h.uc.ConfirmExternal(h.ctx, h.confirmExternalInput("999999", 1))
	requireCerr(t, err, cerr.CodeConflict, "error.otp_already_consumed")
}
