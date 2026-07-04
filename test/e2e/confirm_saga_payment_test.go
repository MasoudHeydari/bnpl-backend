//go:build e2e

package e2e_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

func TestE2E_ConfirmSaga_Q_InitialPaymentPrepareCreatesPaymentAttempt(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.True(t, out.PaymentRequired)
	require.NotNil(t, out.Session.ContractID)

	attempt := h.queryCheckoutPaymentAttempt(*out.Session.ContractID)
	require.Equal(t, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose, attempt.Purpose)
	require.NotEqual(t, model.FailedPaymentAttemptState, attempt.State)
}

func TestE2E_ConfirmSaga_R_CollectionObligationAndCreateCheckoutPaymentIdempotent(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.True(t, out.PaymentRequired)

	contractID := *out.Session.ContractID
	sourceID := contractID.String()
	require.Equal(t, 1, h.fc.callCount(fcCallPrepareCollectionObligation, sourceID))
	require.Equal(t, 1, h.fc.callCount(fcCallCreateCheckoutPayment, sourceID))

	_, err = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	assert.Equal(t, 1, h.fc.callCount(fcCallPrepareCollectionObligation, sourceID))
	assert.Equal(t, 1, h.fc.callCount(fcCallCreateCheckoutPayment, sourceID))
}

func TestE2E_ConfirmSaga_S_LostFCPaymentResponseAfterCommitRecoversOnce(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	var fcPasses atomic.Int32
	var fcClient *controllableCheckoutPaymentClient
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforeFCSubmit: func(context.Context) error {
				if fcPasses.Add(1) == 1 {
					fcClient.setPaymentLostAfterCommit()
				}

				return nil
			},
		},
	})
	fcClient = h.fc
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	_, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	if err != nil {
		h.fc.setPaymentSuccess()
		sagaID := h.findSagaID(seed.Session.ID)
		h.driveSagaUntilIdle(sagaID)
	}

	session := h.querySession(seed.Session.ID)
	require.NotNil(t, session.ContractID)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, session.Status)
	assert.Equal(t, 1, h.fc.committedPaymentCount())
}

func TestE2E_ConfirmSaga_T_ConcurrentConfirmNoDuplicatePaymentSessions(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	}()
	go func() {
		defer wg.Done()
		_, _ = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	}()
	wg.Wait()

	session := h.querySession(seed.Session.ID)
	if session.ContractID == nil {
		h.resumeUntilSessionCompleted(seed.Session.ID)
		session = h.querySession(seed.Session.ID)
	}

	require.NotNil(t, session.ContractID)
	assert.Equal(t, 1, h.fc.committedPaymentCount())
}

func TestE2E_ConfirmSaga_U_PaymentSuccessActivatesContractAndCompletesCheckout(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.True(t, out.PaymentRequired)

	contractRow := h.queryContractBySession(out.Session)
	attempt := h.queryCheckoutPaymentAttempt(contractRow.ID)

	input := h.buildCheckoutPaymentProcessInput(attempt, contractRow, "fc-event-success-1")
	h.processPaymentResult(input)

	session := h.querySession(seed.Session.ID)
	require.Equal(t, model.CompletedCheckoutSessionStatus, session.Status)

	contractRow = h.queryContractBySession(session)
	require.Equal(t, model.ActiveContractStatus, contractRow.Status)
}

func TestE2E_ConfirmSaga_V_BrowserReturnDoesNotActivateCheckout(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.True(t, out.PaymentRequired)

	returnOut, err := h.CheckoutSession.GetReturn(h.ctx, checkoutsessionuc.GetReturnInput{
		CheckoutSessionID:  seed.Session.ID,
		ExternalCustomerID: seed.ExternalCustomerID,
	})
	require.NoError(t, err)
	require.NotEmpty(t, returnOut.ReturnURL)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, returnOut.Session.Status)

	contractRow := h.queryContractBySession(returnOut.Session)
	require.Equal(t, model.PaymentPendingContractStatus, contractRow.Status)
}

func TestE2E_ConfirmSaga_W_DuplicatePaymentSuccessDoesNotDoubleApply(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)

	contractRow := h.queryContractBySession(out.Session)
	attempt := h.queryCheckoutPaymentAttempt(contractRow.ID)

	require.Equal(t, int64(0), h.creditExposureMinor(seed.CustomerID))

	input := h.buildCheckoutPaymentProcessInput(attempt, contractRow, "fc-event-dup-success")
	first := h.processPaymentResult(input)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultPaymentApplied, first.Result)

	replay := h.processPaymentResult(input)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultEventAlreadyProcessed, replay.Result)

	samePayment := h.buildCheckoutPaymentProcessInput(attempt, contractRow, "fc-event-dup-success-other-id")
	second := h.processPaymentResult(samePayment)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultAlreadyApplied, second.Result)

	session := h.querySession(seed.Session.ID)
	require.Equal(t, model.CompletedCheckoutSessionStatus, session.Status)
	require.Equal(t, contractRow.ID, *session.ContractID)

	activated := h.queryContractBySession(session)
	require.Equal(t, model.ActiveContractStatus, activated.Status)
	require.Equal(t, contractRow.FinancialPaymentIntentID, activated.FinancialPaymentIntentID)

	attempts := h.listCheckoutPaymentAttempts(contractRow.ID)
	require.Len(t, attempts, 1)
	require.Equal(t, attempt.ID, attempts[0].ID)
	require.Equal(t, model.SucceededPaymentAttemptState, attempts[0].State)
	require.Equal(t, attempt.IdempotencyKey, attempts[0].IdempotencyKey)
	require.Equal(t, attempt.AmountMinor, attempts[0].AmountMinor)
	require.Equal(t, attempt.Currency, attempts[0].Currency)

	paidCount, paidAmount := h.countPaidInstallments(contractRow.ID)
	require.Equal(t, 1, paidCount)
	require.Equal(t, int64(attempt.AmountMinor), paidAmount)

	exposureCount, exposureAmount := h.countExposureIncreases(contractRow.ID)
	require.Equal(t, 1, exposureCount)
	require.Greater(t, exposureAmount, int64(0))
	require.Equal(t, exposureAmount, h.creditExposureMinor(seed.CustomerID))

	// Merchant lifecycle events are keyed by the public checkout token, never the
	// internal session id. One row after two authoritative replays is the
	// duplicate-completion invariant.
	require.Equal(t, 1, h.countOutboxEvents("bnpl.checkout.completed.v1", session.PublicToken.String()))
	require.Equal(t, 0, h.countOutboxEvents("bnpl.checkout.completed.v1", seed.Session.ID.String()))

	status, inboxCount := h.inboxStatus("financial-core", "fc-event-dup-success")
	require.Equal(t, 1, inboxCount)
	require.Equal(t, "processed", status)
	otherStatus, otherCount := h.inboxStatus("financial-core", "fc-event-dup-success-other-id")
	require.Equal(t, 1, otherCount)
	require.Equal(t, "processed", otherStatus)
}

func TestE2E_ConfirmSaga_X_StaleFailureAfterSuccessDoesNotRegress(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)

	contractRow := h.queryContractBySession(out.Session)
	attempt := h.queryCheckoutPaymentAttempt(contractRow.ID)

	success := h.buildCheckoutPaymentProcessInput(attempt, contractRow, "fc-event-success-first")
	h.processPaymentResult(success)

	failure := h.buildCheckoutPaymentFailureProcessInput(attempt, contractRow, "fc-event-failure-late")
	_, err = h.PaymentResult.Process(h.ctx, failure)
	require.NoError(t, err)

	session := h.querySession(seed.Session.ID)
	require.Equal(t, model.CompletedCheckoutSessionStatus, session.Status)

	contractRow = h.queryContractBySession(session)
	require.Equal(t, model.ActiveContractStatus, contractRow.Status)
}

func TestE2E_ConfirmSaga_Y_TerminalFailureConcurrentRetriesAtMostOneReplacementAttempt(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	var replacementArrivals atomic.Int32

	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		beforeTerminalCheckoutPaymentReplacement: func(context.Context) error {
			arrived <- struct{}{}
			replacementArrivals.Add(1)
			<-release
			return nil
		},
	})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.True(t, out.PaymentRequired)

	contractRow := h.queryContractBySession(out.Session)
	require.Equal(t, model.PaymentPendingContractStatus, contractRow.Status)
	require.Equal(t, 1, h.countContractsForDecision(*out.Session.DecisionID))
	attemptA := h.queryCheckoutPaymentAttempt(contractRow.ID)
	require.Equal(t, model.SubmittedPaymentAttemptState, attemptA.State)
	require.Equal(t, 1, h.countConfirmationSagasForSession(seed.Session.ID))
	originalFCPayments := h.fc.committedPaymentCount()
	require.Equal(t, 1, originalFCPayments)

	failure := h.buildCheckoutPaymentFailureProcessInput(attemptA, contractRow, "fc-event-terminal-failure")
	failureOut := h.processPaymentResult(failure)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultFailureRecorded, failureOut.Result)

	failed := h.listCheckoutPaymentAttempts(contractRow.ID)
	require.Len(t, failed, 1)
	require.Equal(t, attemptA.ID, failed[0].ID)
	require.Equal(t, model.FailedPaymentAttemptState, failed[0].State)
	require.Equal(t, attemptA.IdempotencyKey, failed[0].IdempotencyKey)

	reopened := h.querySession(seed.Session.ID)
	require.Equal(t, model.OpenCheckoutSessionStatus, reopened.Status)
	require.NotNil(t, reopened.ContractID)
	require.Equal(t, contractRow.ID, *reopened.ContractID)
	require.Equal(t, model.PaymentPendingContractStatus, h.queryContractBySession(reopened).Status)

	confirmIn := confirmInputForSeed(seed)
	var (
		wg       sync.WaitGroup
		errA     error
		errB     error
		confirmA checkoutsessionuc.ConfirmOutput
		confirmB checkoutsessionuc.ConfirmOutput
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		confirmA, errA = h.CheckoutSession.Confirm(h.ctx, confirmIn)
	}()
	go func() {
		defer wg.Done()
		confirmB, errB = h.CheckoutSession.Confirm(h.ctx, confirmIn)
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-arrived:

		case <-time.After(20 * time.Second):
			t.Fatal("concurrent replacement barrier was not reached by both Confirm calls")
		}
	}
	close(release)
	wg.Wait()

	require.Equal(t, int32(2), replacementArrivals.Load(), "both confirms must overlap at replacement creation")
	assertAcceptablePaymentRetryConfirm(t, errA)
	assertAcceptablePaymentRetryConfirm(t, errB)
	require.True(
		t,
		errA == nil || errB == nil,
		"at least one retry Confirm must progress: a=%v b=%v",
		errA,
		errB,
	)

	session := h.querySession(seed.Session.ID)
	require.Equal(t, contractRow.ID, *session.ContractID)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, session.Status)
	require.Equal(t, 1, h.countContractsForDecision(*session.DecisionID))
	require.Equal(t, 1, h.countConfirmationSagasForSession(seed.Session.ID))

	attempts := h.listCheckoutPaymentAttempts(contractRow.ID)
	require.Len(t, attempts, 2)
	var (
		original    checkoutPaymentAttemptSnapshot
		replacement checkoutPaymentAttemptSnapshot
		foundA      bool
		foundB      bool
	)
	for _, row := range attempts {
		if row.ID == attemptA.ID {
			original = row
			foundA = true
			continue
		}

		replacement = row
		foundB = true
	}
	require.True(t, foundA)
	require.True(t, foundB)
	require.Equal(t, model.FailedPaymentAttemptState, original.State)
	require.Equal(t, attemptA.IdempotencyKey, original.IdempotencyKey)
	require.NotEqual(t, attemptA.ID, replacement.ID)
	require.NotEqual(t, attemptA.IdempotencyKey, replacement.IdempotencyKey)
	require.Equal(t, model.SubmittedPaymentAttemptState, replacement.State)
	require.Equal(t, attemptA.AmountMinor, replacement.AmountMinor)
	require.Equal(t, attemptA.Currency, replacement.Currency)
	require.Equal(t, contractRow.PayableNowAmountMinor, replacement.AmountMinor)
	require.Equal(t, contractRow.Currency, replacement.Currency)

	keys := h.fc.committedPaymentKeys()
	require.Len(t, keys, 2)
	require.Contains(t, keys, attemptA.IdempotencyKey)
	require.Contains(t, keys, replacement.IdempotencyKey)
	require.Equal(t, 2, h.fc.committedPaymentCount())

	if errA == nil {
		require.Equal(t, contractRow.ID, confirmA.Contract.ID)
		require.Equal(t, replacement.IdempotencyKey, confirmA.PaymentAttempt.IdempotencyKey)
	}

	if errB == nil {
		require.Equal(t, contractRow.ID, confirmB.Contract.ID)
		require.Equal(t, replacement.IdempotencyKey, confirmB.PaymentAttempt.IdempotencyKey)
	}
}

func assertAcceptablePaymentRetryConfirm(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		return
	}

	var appErr *cerr.Error
	require.ErrorAs(t, err, &appErr)
	switch appErr.MessageKey {
	case "error.checkout_payment_is_in_progress_and_must_be_resolved_first",
		"error.checkout_payment_attempt_superseded_by_newer_retry":
		return

	default:
		t.Fatalf("retry Confirm returned %s (%s)", appErr.Code, appErr.MessageKey)
	}
}
