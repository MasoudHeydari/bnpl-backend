//go:build e2e

package e2e_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extnotification "github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

type recordingNotificationSubmitter struct {
	mu    sync.Mutex
	calls []extnotification.SubmitInput
}

func (r *recordingNotificationSubmitter) Submit(
	_ context.Context,
	input extnotification.SubmitInput,
) (extnotification.SubmitOutput, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.calls = append(r.calls, input)

	return extnotification.SubmitOutput{
		RequestID: "e2e-notif-" + input.IdempotencyKey,
		Duplicate: false,
	}, nil
}

func (r *recordingNotificationSubmitter) lastCode(t *testing.T) string {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.calls)
	code := r.calls[len(r.calls)-1].Variables["code"]
	require.NotEmpty(t, code)
	return code
}

func (r *recordingNotificationSubmitter) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func TestE2E_OTP_StrictlySingleUseAndIssuanceWindow(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	recorder := &recordingNotificationSubmitter{}
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID:    externalCustomerID,
		notificationSubmitter: recorder,
	})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	startOut, err := h.CheckoutSession.StartExternal(h.ctx, checkoutsessionuc.StartExternalInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)
	require.Equal(t, checkoutsessionuc.ApprovedOfferOutcome, startOut.Offer.Outcome)
	require.Equal(t, 0, recorder.callCount())

	otpOut, err := h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)
	require.True(t, otpOut.OTPSent)
	require.NotNil(t, otpOut.OTPExpiresAt)
	require.NotNil(t, otpOut.OTPNextIssueAt)
	require.Equal(t, otpOut.OTPExpiresAt.UTC(), otpOut.OTPNextIssueAt.UTC())
	require.Equal(t, checkoutsessionuc.OTPAvailabilityAwaitingEntry, otpOut.OTPAvailability)
	require.Equal(t, 1, recorder.callCount())
	codeA := recorder.lastCode(t)

	confirmOut, err := h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
		Token:                seed.Session.PublicToken,
		OTP:                  codeA,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)
	require.True(t, confirmOut.PaymentRequired)
	require.NotNil(t, confirmOut.Session.ContractID)

	afterConsume := h.queryOTPChallenge(seed.Session.ID)
	require.NotNil(t, afterConsume.VerifiedAt)
	require.Nil(t, afterConsume.Verifier)
	require.Nil(t, afterConsume.ExpiresAt)
	require.NotNil(t, afterConsume.NextIssueAt)
	require.Equal(t, otpOut.OTPNextIssueAt.UTC(), afterConsume.NextIssueAt.UTC())

	_, err = h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
		Token:                seed.Session.PublicToken,
		OTP:                  codeA,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.Error(t, err)
	var appErr *cerr.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, cerr.CodeConflict, appErr.Code)
	require.Equal(t, cerr.MessageKey("error.otp_already_consumed"), appErr.MessageKey)

	_, err = h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
		Token:                seed.Session.PublicToken,
		OTP:                  "000000",
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.Error(t, err)
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, cerr.MessageKey("error.otp_already_consumed"), appErr.MessageKey)

	status, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, checkoutsessionuc.OTPAvailabilityConsumedWait, status.OTPAvailability)
	require.NotNil(t, status.OTPNextIssueAt)

	_, err = h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeTooManyRequests))
	require.Equal(t, 1, recorder.callCount())

	h.clock.Time = afterConsume.NextIssueAt.Add(-time.Nanosecond)
	_, err = h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeTooManyRequests))
	require.Equal(t, 1, recorder.callCount())

	h.clock.Time = afterConsume.NextIssueAt.UTC()
	contOut, err := h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)
	require.True(t, contOut.OTPSent)
	require.Equal(t, 2, recorder.callCount())
	codeB := recorder.lastCode(t)
	require.NotEqual(t, codeA, codeB)

	replay, err := h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
		Token:                seed.Session.PublicToken,
		OTP:                  codeB,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)
	require.Equal(t, *confirmOut.Session.ContractID, *replay.Session.ContractID)
	require.Equal(t, 1, h.countCheckoutPaymentAttempts(*replay.Session.ContractID))
	require.Equal(t, 1, h.fc.committedPaymentCount())
}

func TestE2E_OTP_OnStartStatusIsNotReconfirm(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})

	customerID := insertCustomerProfile(t, externalCustomerID, model.ActiveCustomerStatus)
	insertCreditAccount(t, customerID, 90_000_000)
	insertCheckoutPolicyV2Assignment(t, externalCustomerID)

	merchantID := model.MerchantID(uuid.New())
	onlineStoreID := model.OnlineStoreID(uuid.New())
	insertMerchantOnlineStackOnStart(t, merchantID, onlineStoreID, confirmSagaReturnDest)

	out, err := h.CheckoutSession.CreatePartner(h.ctx, checkoutsessionuc.CreatePartnerInput{
		CheckoutSessionRequest: checkoutsessionuc.CheckoutSessionRequest{
			MerchantID:           merchantID,
			OnlineStoreID:        onlineStoreID,
			MerchantOrderID:      "order-" + uuid.NewString(),
			IdempotencyKey:       "idem-" + uuid.NewString(),
			CustomerReference:    "cust-ref",
			ReturnDestination:    confirmSagaReturnDest,
			MerchantState:        "state",
			RequestedAmountMinor: confirmSagaTestAmountMinor,
			Currency:             model.IRRCurrency,
		},
		CustomerPhone: "+989121111111",
	})
	require.NoError(t, err)

	session := h.querySession(out.Session.ID)
	require.Equal(t, 1, session.EvaluationGeneration)
	require.Nil(t, session.DecisionID)

	status, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, checkoutsessionuc.ConfirmationStatusNotStarted, status.ConfirmationStatus)
	require.Equal(t, checkoutsessionuc.OTPAvailabilityIssuanceBlocked, status.OTPAvailability)
}

func TestE2E_OTP_ConcurrentConfirmConsumesExactlyOnce(t *testing.T) {
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
	code := recorder.lastCode(t)

	const goroutines = 2
	startBarrier := make(chan struct{})
	results := make(chan error, goroutines)

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startBarrier
			_, confirmErr := h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
				Token:                seed.Session.PublicToken,
				OTP:                  code,
				EvaluationGeneration: seed.Session.EvaluationGeneration,
			})
			results <- confirmErr
		}()
	}

	close(startBarrier)
	wg.Wait()
	close(results)

	var successes, consumedConflicts int
	var other []error
	for err := range results {
		if err == nil {
			successes++
			continue
		}

		var appErr *cerr.Error
		if !errors.As(err, &appErr) {
			other = append(other, err)
			continue
		}

		if appErr.MessageKey == cerr.MessageKey("error.otp_already_consumed") {
			consumedConflicts++
			continue
		}

		other = append(other, err)
	}

	require.Empty(t, other, "unexpected confirm errors: %v", other)
	require.Equal(t, 1, successes)
	require.Equal(t, 1, consumedConflicts)

	challenge := h.queryOTPChallenge(seed.Session.ID)
	require.NotNil(t, challenge.VerifiedAt)
	require.Nil(t, challenge.Verifier)
	session := h.querySession(seed.Session.ID)
	require.Equal(t, 1, h.fc.committedPaymentCount())
	require.NotNil(t, session.ContractID)
	require.Equal(t, 1, h.countCheckoutPaymentAttempts(*session.ContractID))
}

func TestE2E_OTP_ConcurrentIssuanceAtDeadlineCreatesAtMostOneSMS(t *testing.T) {
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
	require.Equal(t, 0, recorder.callCount())

	otpOut, err := h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)
	require.Equal(t, 1, recorder.callCount())

	_, err = h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
		Token:                seed.Session.PublicToken,
		OTP:                  recorder.lastCode(t),
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)

	after := h.queryOTPChallenge(seed.Session.ID)
	require.NotNil(t, after.NextIssueAt)
	require.Equal(t, otpOut.OTPNextIssueAt.UTC(), after.NextIssueAt.UTC())

	h.clock.Time = after.NextIssueAt.UTC()

	const goroutines = 2
	startBarrier := make(chan struct{})
	type issueResult struct {
		err     error
		otpSent bool
	}
	results := make(chan issueResult, goroutines)

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startBarrier
			out, issueErr := h.CheckoutSession.RequestExternalOTP(
				h.ctx,
				checkoutsessionuc.RequestExternalOTPInput{
					Token:                seed.Session.PublicToken,
					EvaluationGeneration: seed.Session.EvaluationGeneration,
				},
			)
			results <- issueResult{err: issueErr, otpSent: out.OTPSent}
		}()
	}

	close(startBarrier)
	wg.Wait()
	close(results)

	var successes, smsSent int
	for result := range results {
		require.NoError(t, result.err)
		successes++
		if result.otpSent {
			smsSent++
		}
	}

	require.Equal(t, 2, successes)
	require.Equal(t, 1, smsSent)
	require.Equal(t, 2, recorder.callCount())

	challenge := h.queryOTPChallenge(seed.Session.ID)
	require.NotNil(t, challenge.Verifier)
	require.Equal(t, 2, challenge.SendCount)
}

func TestE2E_OTP_ManualReviewBlocksIssuanceAndConfirmDoesNotConsume(t *testing.T) {
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

	otpOut, err := h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)
	require.True(t, otpOut.OTPSent)
	code := recorder.lastCode(t)

	confirmOut, err := h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
		Token:                seed.Session.PublicToken,
		OTP:                  code,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)
	require.NotNil(t, confirmOut.Session.ContractID)

	attempt := h.queryCheckoutPaymentAttempt(*confirmOut.Session.ContractID)
	_, err = testDB.Exec(h.ctx, `
		UPDATE bnpl.payment_attempts
		SET state = $2,
			manual_review_required_at = now(),
			last_reconciliation_error = 'e2e-manual-review',
			next_reconciliation_at = NULL,
			updated_at = now()
		WHERE id = $1
	`, attempt.ID, model.ManualReviewRequiredPaymentAttemptState)
	require.NoError(t, err)

	status, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, checkoutsessionuc.ConfirmationStatusNeedsAttention, status.ConfirmationStatus)
	require.Equal(t, checkoutsessionuc.OTPAvailabilityIssuanceBlocked, status.OTPAvailability)

	beforeSMS := recorder.callCount()
	h.clock.Time = h.queryOTPChallenge(seed.Session.ID).NextIssueAt.UTC()
	_, err = h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.Error(t, err)
	var appErr *cerr.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, cerr.MessageKey("error.checkout_otp_issuance_not_allowed"), appErr.MessageKey)
	require.Equal(t, beforeSMS, recorder.callCount())

	// Re-open issuance by clearing the PA block, issue a continuation OTP, then
	// flip PA back to manual_review before confirm so ConsumeAuthorizer aborts
	// without burning the challenge.
	_, err = testDB.Exec(h.ctx, `
		UPDATE bnpl.payment_attempts
		SET state = $2,
			manual_review_required_at = NULL,
			last_reconciliation_error = NULL,
			updated_at = now()
		WHERE id = $1
	`, attempt.ID, model.CreatedPaymentAttemptState)
	require.NoError(t, err)

	contOut, err := h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)
	require.True(t, contOut.OTPSent)
	codeB := recorder.lastCode(t)
	challengeBefore := h.queryOTPChallenge(seed.Session.ID)
	require.NotNil(t, challengeBefore.Verifier)

	_, err = testDB.Exec(h.ctx, `
		UPDATE bnpl.payment_attempts
		SET state = $2,
			manual_review_required_at = now(),
			last_reconciliation_error = 'e2e-manual-review',
			next_reconciliation_at = NULL,
			updated_at = now()
		WHERE id = $1
	`, attempt.ID, model.ManualReviewRequiredPaymentAttemptState)
	require.NoError(t, err)

	_, err = h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
		Token:                seed.Session.PublicToken,
		OTP:                  codeB,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.Error(t, err)
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, cerr.MessageKey("error.checkout_otp_issuance_not_allowed"), appErr.MessageKey)

	challengeAfter := h.queryOTPChallenge(seed.Session.ID)
	require.Nil(t, challengeAfter.VerifiedAt)
	require.NotNil(t, challengeAfter.Verifier)
	require.Equal(t, *challengeBefore.IssueID, *challengeAfter.IssueID)
}

func TestE2E_OTP_IssuanceVsSagaStepNoDeadlock(t *testing.T) {
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

	_, err = h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
		Token:                seed.Session.PublicToken,
		OTP:                  recorder.lastCode(t),
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.NoError(t, err)

	after := h.queryOTPChallenge(seed.Session.ID)
	require.NotNil(t, after.NextIssueAt)
	h.clock.Time = after.NextIssueAt.UTC()

	entered := make(chan struct{})
	release := make(chan struct{})
	h.OTP.SetBeforeIssueCommit(func(context.Context) error {
		close(entered)
		<-release
		return nil
	})

	type issueResult struct {
		err error
	}
	issueDone := make(chan issueResult, 1)
	go func() {
		_, issueErr := h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
			Token:                seed.Session.PublicToken,
			EvaluationGeneration: seed.Session.EvaluationGeneration,
		})
		issueDone <- issueResult{err: issueErr}
	}()

	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("OTP issuance did not reach beforeIssueCommit barrier")
	}

	sagaID := h.findSagaID(seed.Session.ID)
	worker := h.newWorkerExecutor("e2e-otp-lock-order")
	execDone := make(chan error, 1)
	go func() {
		execDone <- worker.ExecuteByID(h.ctx, sagaID)
	}()

	// Give the saga worker a moment to contend on overlapping rows; with the
	// canonical Saga→…→CS order this must not 40P01.
	time.Sleep(200 * time.Millisecond)
	close(release)

	issue := <-issueDone
	require.NoError(t, issue.err)

	select {
	case execErr := <-execDone:
		require.False(t, containsDeadlock(execErr), "deadlock: %v", execErr)

	case <-time.After(15 * time.Second):
		t.Fatal("saga ExecuteByID timed out — likely lock-order deadlock")
	}
}

func containsDeadlock(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "40P01") || strings.Contains(strings.ToLower(msg), "deadlock")
}

func TestE2E_OTP_SagaFirstLockOrderNoDeadlock(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	recorder := &recordingNotificationSubmitter{}

	entered := make(chan struct{})
	release := make(chan struct{})
	var enterOnce sync.Once
	var armBarrier atomic.Bool

	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID:    externalCustomerID,
		notificationSubmitter: recorder,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			// Fires after Saga FOR UPDATE and before CheckoutSession lock.
			BeforePaymentSubmitted: func(context.Context) error {
				if !armBarrier.Load() {
					return nil
				}

				enterOnce.Do(func() { close(entered) })
				<-release

				return nil
			},
		},
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
	code := recorder.lastCode(t)

	armBarrier.Store(true)

	confirmDone := make(chan error, 1)
	go func() {
		_, confirmErr := h.CheckoutSession.ConfirmExternal(h.ctx, checkoutsessionuc.ConfirmExternalInput{
			Token:                seed.Session.PublicToken,
			OTP:                  code,
			EvaluationGeneration: seed.Session.EvaluationGeneration,
		})
		confirmDone <- confirmErr
	}()

	select {
	case <-entered:
	case <-time.After(20 * time.Second):
		t.Fatal("saga did not reach BeforePaymentSubmitted with Saga lock held")
	}

	issueDone := make(chan error, 1)
	go func() {
		_, issueErr := h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
			Token:                seed.Session.PublicToken,
			EvaluationGeneration: seed.Session.EvaluationGeneration,
		})
		issueDone <- issueErr
	}()

	close(release)

	select {
	case confirmErr := <-confirmDone:
		require.NoError(t, confirmErr)
		require.False(t, containsDeadlock(confirmErr))

	case <-time.After(20 * time.Second):
		t.Fatal("ConfirmExternal timed out — likely lock-order deadlock")
	}

	select {
	case issueErr := <-issueDone:
		// Window-active / business rejection is fine; 40P01 is not.
		require.False(t, containsDeadlock(issueErr), "deadlock: %v", issueErr)

	case <-time.After(20 * time.Second):
		t.Fatal("RequestExternalOTP timed out — likely lock-order deadlock")
	}
}

func TestE2E_OTP_ExpiredPaymentSessionBlocksIssuance(t *testing.T) {
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
	fcCallsBefore := h.fc.totalCallCount()
	smsBefore := recorder.callCount()
	challengeBefore := h.queryOTPChallenge(seed.Session.ID)
	sendBefore := challengeBefore.SendCount

	// Expire the durable gateway link without inventing a financial failure.
	expiredAt := h.clock.Now().UTC().Add(-time.Second)
	_, err = testDB.Exec(h.ctx, `
		UPDATE bnpl.contracts
		SET financial_payment_expires_at = $2, updated_at = now()
		WHERE id = $1
	`, contractRow.ID, expiredAt)
	require.NoError(t, err)
	_, err = testDB.Exec(h.ctx, `
		UPDATE bnpl.payment_attempts
		SET financial_payment_expires_at = $2, updated_at = now()
		WHERE id = $1
	`, attempt.ID, expiredAt)
	require.NoError(t, err)

	require.NotNil(t, challengeBefore.NextIssueAt)
	h.clock.Time = challengeBefore.NextIssueAt.UTC()

	status, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, checkoutsessionuc.OTPAvailabilityIssuanceBlocked, status.OTPAvailability)
	require.NotEqual(t, checkoutsessionuc.ConfirmationNextActionResumePayment, status.NextAction)

	_, err = h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.Error(t, err)
	var appErr *cerr.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, cerr.MessageKey("error.checkout_otp_issuance_not_allowed"), appErr.MessageKey)

	require.Equal(t, smsBefore, recorder.callCount())
	require.Equal(t, sendBefore, h.queryOTPChallenge(seed.Session.ID).SendCount)
	require.Equal(t, fcCallsBefore, h.fc.totalCallCount())

	attemptAfter := h.queryCheckoutPaymentAttempt(*confirmOut.Session.ContractID)
	require.Equal(t, attempt.ID, attemptAfter.ID)
	require.NotEqual(t, model.FailedPaymentAttemptState, attemptAfter.State)
}

func TestE2E_OTP_MissingConfirmationSagaFailsClosed(t *testing.T) {
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
	require.NotNil(t, confirmOut.Session.ConfirmationAttemptID)

	// Orphan the session's ConfirmationAttemptID so lookup finds no Saga for the
	// current attempt — without deleting historical saga rows (FK-protected).
	orphanAttempt := uuid.New()
	_, err = testDB.Exec(h.ctx, `
		UPDATE bnpl.checkout_sessions
		SET confirmation_attempt_id = $2, updated_at = now()
		WHERE id = $1
	`, seed.Session.ID, orphanAttempt)
	require.NoError(t, err)

	status, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, checkoutsessionuc.ConfirmationStatusNeedsAttention, status.ConfirmationStatus)
	require.Equal(t, checkoutsessionuc.OTPAvailabilityIssuanceBlocked, status.OTPAvailability)

	smsBefore := recorder.callCount()
	challenge := h.queryOTPChallenge(seed.Session.ID)
	require.NotNil(t, challenge.NextIssueAt)
	h.clock.Time = challenge.NextIssueAt.UTC()

	_, err = h.CheckoutSession.RequestExternalOTP(h.ctx, checkoutsessionuc.RequestExternalOTPInput{
		Token:                seed.Session.PublicToken,
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	})
	require.Error(t, err)
	var appErr *cerr.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, cerr.MessageKey("error.checkout_otp_issuance_not_allowed"), appErr.MessageKey)
	require.Equal(t, smsBefore, recorder.callCount())
}
