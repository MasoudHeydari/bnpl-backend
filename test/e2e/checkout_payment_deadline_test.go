//go:build e2e

package e2e_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

// TestE2E_CheckoutDeadline_BNPLBeforeFCExpiryBlocksContinuation covers the
// mandatory regression where FC PaymentSession outlives Contract.ExpiresAt.
func TestE2E_CheckoutDeadline_BNPLBeforeFCExpiryBlocksContinuation(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	recorder := &recordingNotificationSubmitter{}
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID:    externalCustomerID,
		notificationSubmitter: recorder,
	})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	_, err := h.CheckoutSession.StartExternal(h.ctx, checkoutsessionuc.StartExternalInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)

	_, err = h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)

	confirmOut, err := h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
		Token:                seed.Session.PublicToken,
		OTP:                  recorder.lastCode(t),
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)
	require.NotNil(t, confirmOut.Session.ContractID)

	contractRow := h.queryContractBySession(confirmOut.Session)
	require.NotNil(t, contractRow.FinancialPaymentExpiresAt)
	attempt := h.queryCheckoutPaymentAttempt(*confirmOut.Session.ContractID)
	require.NotNil(t, attempt.FinancialPaymentExpiresAt)

	bnplDeadline := h.clock.Now().UTC().Add(15 * time.Minute)
	fcLater := bnplDeadline.Add(30 * time.Minute)
	_, err = testDB.Exec(h.ctx, `
		UPDATE bnpl.contracts
		SET expires_at = $2,
		    financial_payment_expires_at = $3,
		    updated_at = now()
		WHERE id = $1
	`, contractRow.ID, bnplDeadline, fcLater)
	require.NoError(t, err)
	_, err = testDB.Exec(h.ctx, `
		UPDATE bnpl.payment_attempts
		SET financial_payment_expires_at = $2, updated_at = now()
		WHERE id = $1
	`, attempt.ID, fcLater)
	require.NoError(t, err)

	reloaded := h.queryContractBySession(confirmOut.Session)
	require.True(t, reloaded.ExpiresAt.UTC().Equal(bnplDeadline))
	require.NotNil(t, reloaded.FinancialPaymentExpiresAt)
	require.True(t, reloaded.FinancialPaymentExpiresAt.UTC().Equal(fcLater))
	require.Equal(t, bnplDeadline, reloaded.EffectivePaymentDeadline())

	// Still before BNPL deadline: resume remains available.
	statusBefore, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, checkoutsessionuc.ConfirmationNextActionResumePayment, statusBefore.NextAction)

	// Advance logical clock past BNPL deadline while FC link remains valid.
	h.clock.Time = bnplDeadline.Add(time.Second)
	fcCallsBefore := h.fc.totalCallCount()
	smsBefore := recorder.callCount()

	statusAfter, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.NotEqual(t, checkoutsessionuc.ConfirmationNextActionResumePayment, statusAfter.NextAction)
	require.NotEqual(t, checkoutsessionuc.ConfirmationNextActionRetryPayment, statusAfter.NextAction)
	require.Equal(t, checkoutsessionuc.OTPAvailabilityIssuanceBlocked, statusAfter.OTPAvailability)

	_, err = h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.Error(t, err)
	var appErr *cerr.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, cerr.MessageKey("error.checkout_otp_issuance_not_allowed"), appErr.MessageKey)
	require.Equal(t, smsBefore, recorder.callCount())
	require.Equal(t, fcCallsBefore, h.fc.totalCallCount())

	// Authoritative FC failure while past the BNPL deadline must not reopen
	// retry_payment or create a replacement PaymentAttempt / FC session.
	createCallsBeforeFailure := h.fc.callCount(fcCallCreateCheckoutPayment, "")
	require.Equal(t, 1, h.fc.committedPaymentCount())

	failedAttempt := h.queryCheckoutPaymentAttempt(*confirmOut.Session.ContractID)
	failedContract := h.queryContractBySession(confirmOut.Session)
	failure := h.buildCheckoutPaymentFailureProcessInput(
		failedAttempt,
		failedContract,
		"fc-event-deadline-retry-blocked",
	)
	h.processPaymentResult(failure)

	retryStatus, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.NotEqual(t, checkoutsessionuc.ConfirmationNextActionRetryPayment, retryStatus.NextAction)

	_, err = h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
		Token:                seed.Session.PublicToken,
		OTP:                  "000000",
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.Error(t, err)
	require.Equal(t, createCallsBeforeFailure, h.fc.callCount(fcCallCreateCheckoutPayment, ""))
	require.Equal(t, 1, h.fc.committedPaymentCount())
	require.Equal(t, 1, h.countCheckoutPaymentAttempts(contractRow.ID))

	storedFC := h.queryContractBySession(confirmOut.Session)
	require.NotNil(t, storedFC.FinancialPaymentExpiresAt)
	require.True(t, storedFC.FinancialPaymentExpiresAt.UTC().Equal(fcLater),
		"durable FC expiry must remain the original FC value")
}

func seedPaymentPendingCheckoutForDeadline(
	t *testing.T,
) (*confirmSagaHarness, model.CheckoutSession, model.Contract, model.PaymentAttempt, time.Time) {
	t.Helper()

	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.True(t, out.PaymentRequired)

	session := h.querySession(seed.Session.ID)
	contractRow := h.queryContractBySession(session)
	attempt := h.queryCheckoutPaymentAttempt(contractRow.ID)
	require.Equal(t, model.SubmittedPaymentAttemptState, attempt.State)
	require.NotNil(t, attempt.FinancialPaymentIntentID)

	bnplDeadline := h.clock.Now().UTC().Add(15 * time.Minute)
	_, err = testDB.Exec(h.ctx, `
		UPDATE bnpl.contracts
		SET expires_at = $2, updated_at = now()
		WHERE id = $1
	`, contractRow.ID, bnplDeadline)
	require.NoError(t, err)

	contractRow = h.queryContractBySession(session)
	require.True(t, contractRow.ExpiresAt.UTC().Equal(bnplDeadline))

	return h, session, contractRow, attempt, bnplDeadline
}

// TestE2E_CheckoutDeadline_OnTimePaidAtAcceptedAfterLateEventDelivery verifies that an
// authoritative FC success with paid_at before the BNPL deadline is applied even when
// the event arrives after the deadline.
func TestE2E_CheckoutDeadline_OnTimePaidAtAcceptedAfterLateEventDelivery(t *testing.T) {
	h, session, contractRow, attempt, bnplDeadline := seedPaymentPendingCheckoutForDeadline(t)

	paidAt := bnplDeadline.Add(-time.Minute)
	h.clock.Time = bnplDeadline.Add(15 * time.Minute)

	input := h.buildCheckoutPaymentProcessInput(attempt, contractRow, "fc-event-ontime-late-delivery")
	input.PaidAt = paidAt
	input.EventOccurredAt = h.clock.Now().UTC()
	h.fc.setPaymentAuthoritativeSucceeded(
		*attempt.FinancialPaymentIntentID,
		attempt.AmountMinor,
		string(attempt.Currency),
		paidAt,
	)

	out := h.processPaymentResult(input)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultPaymentApplied, out.Result)

	completed := h.querySession(session.ID)
	require.Equal(t, model.CompletedCheckoutSessionStatus, completed.Status)

	finalContract := h.queryContractBySession(completed)
	require.Equal(t, model.ActiveContractStatus, finalContract.Status)

	finalAttempt := h.queryCheckoutPaymentAttempt(contractRow.ID)
	require.Equal(t, model.SucceededPaymentAttemptState, finalAttempt.State)

	inboxStatus, inboxCount := h.inboxStatus("financial-core", "fc-event-ontime-late-delivery")
	require.Equal(t, 1, inboxCount)
	require.Equal(t, "processed", inboxStatus)
}

// TestE2E_CheckoutDeadline_LatePaidAtFollowsExistingRecoveryPolicy verifies that an
// authoritative FC success with paid_at after the BNPL deadline is not treated as a
// payment failure; existing late-payment reconciliation policy applies.
func TestE2E_CheckoutDeadline_LatePaidAtFollowsExistingRecoveryPolicy(t *testing.T) {
	h, session, contractRow, attempt, bnplDeadline := seedPaymentPendingCheckoutForDeadline(t)

	paidAt := bnplDeadline.Add(5 * time.Minute)
	h.clock.Time = bnplDeadline.Add(15 * time.Minute)

	input := h.buildCheckoutPaymentProcessInput(attempt, contractRow, "fc-event-late-paid-at")
	input.PaidAt = paidAt
	input.EventOccurredAt = h.clock.Now().UTC()
	h.fc.setPaymentAuthoritativeSucceeded(
		*attempt.FinancialPaymentIntentID,
		attempt.AmountMinor,
		string(attempt.Currency),
		paidAt,
	)

	out := h.processPaymentResult(input)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultReconciliationRequired, out.Result)

	pending := h.querySession(session.ID)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, pending.Status)

	finalContract := h.queryContractBySession(pending)
	require.Equal(t, model.PaymentPendingContractStatus, finalContract.Status)

	finalAttempt := h.queryCheckoutPaymentAttempt(contractRow.ID)
	require.Equal(t, model.ReconciliationRequiredPaymentAttemptState, finalAttempt.State)
	require.NotEqual(t, model.FailedPaymentAttemptState, finalAttempt.State)
	require.NotNil(t, finalAttempt.ReconciliationReason)
	require.Equal(t, model.PaymentAfterDeadlinePaymentReconciliationReason, *finalAttempt.ReconciliationReason)
	require.NotNil(t, finalAttempt.FinancialPaymentIntentID)
	require.Equal(t, *attempt.FinancialPaymentIntentID, *finalAttempt.FinancialPaymentIntentID)

	inboxStatus, inboxCount := h.inboxStatus("financial-core", "fc-event-late-paid-at")
	require.Equal(t, 1, inboxCount)
	require.Equal(t, "reconciliation_required", inboxStatus)
}
