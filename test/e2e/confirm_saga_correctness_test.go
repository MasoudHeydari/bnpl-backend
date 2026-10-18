//go:build e2e

package e2e_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

func TestE2E_ConfirmSaga_CompensationRejectsStaleGenerationConfirm(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	var customerID model.CustomerID
	enteredPrepare := make(chan struct{})
	releasePrepare := make(chan struct{})
	enteredResume := make(chan struct{})
	releaseResume := make(chan struct{})
	var prepared atomic.Bool
	var resumed atomic.Bool

	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforePrepareContract: func(context.Context) error {
				if !prepared.CompareAndSwap(false, true) || customerID.IsZero() {
					return nil
				}

				updateCreditAccountLimit(t, customerID, 0)
				close(enteredPrepare)
				<-releasePrepare

				return nil
			},
			OnResumeConfirming: func(context.Context) error {
				if !resumed.CompareAndSwap(false, true) {
					return nil
				}

				close(enteredResume)
				<-releaseResume

				return nil
			},
		},
	})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)
	customerID = seed.CustomerID
	generationG1 := seed.Session.EvaluationGeneration
	decisionG1 := *seed.Session.DecisionID

	var (
		outA checkoutsessionuc.ConfirmOutput
		errA error
		outB checkoutsessionuc.ConfirmOutput
		errB error
	)
	doneA := make(chan struct{})
	go func() {
		defer close(doneA)
		outA, errA = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	}()

	select {
	case <-enteredPrepare:
	case <-time.After(20 * time.Second):
		t.Fatal("confirmation did not reach contract preparation")
	}

	doneB := make(chan struct{})
	go func() {
		defer close(doneB)
		outB, errB = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	}()

	select {
	case <-enteredResume:
	case <-time.After(20 * time.Second):
		t.Fatal("stale Confirm did not overlap the confirming claim")
	}

	close(releasePrepare)
	select {
	case <-doneA:
	case <-time.After(20 * time.Second):
		t.Fatal("compensating Confirm did not finish")
	}

	require.NoError(t, errA)
	require.True(t, outA.ReconfirmRequired)
	compensated := h.querySession(seed.Session.ID)
	require.Equal(t, model.OpenCheckoutSessionStatus, compensated.Status)
	require.Nil(t, compensated.ContractID)
	require.True(t, compensated.ConfirmationAttemptID == nil || compensated.ConfirmationAttemptID.IsZero())
	require.Greater(t, compensated.EvaluationGeneration, generationG1)
	require.Equal(t, 0, h.countContractsForDecision(decisionG1))
	require.Equal(t, 0, h.fc.committedEntitlementCount())
	require.Equal(t, 1, h.countConfirmationSagasForSession(seed.Session.ID))

	oldSaga := h.oldestConfirmationSagaForSession(seed.Session.ID)
	require.Equal(t, model.CompensatedSagaStatus, oldSaga.Status)

	close(releaseResume)
	select {
	case <-doneB:
	case <-time.After(20 * time.Second):
		t.Fatal("overlapping Confirm did not finish")
	}

	require.NoError(t, errB)
	require.True(t, outB.ReconfirmRequired)
	afterOverlap := h.querySession(seed.Session.ID)
	require.Equal(t, model.OpenCheckoutSessionStatus, afterOverlap.Status)
	require.Nil(t, afterOverlap.ContractID)
	require.Equal(t, 1, h.countConfirmationSagasForSession(seed.Session.ID))
	require.Equal(t, 0, h.countContractsForDecision(decisionG1))

	updateCreditAccountLimit(t, seed.CustomerID, 90_000_000)

	stale, staleErr := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, staleErr)
	require.True(t, stale.ReconfirmRequired)
	require.Greater(t, stale.Session.EvaluationGeneration, generationG1)
	require.Nil(t, h.querySession(seed.Session.ID).ContractID)
	require.Equal(t, 1, h.countConfirmationSagasForSession(seed.Session.ID))

	progressed, progressErr := h.CheckoutSession.Confirm(h.ctx, checkoutsessionuc.ConfirmInput{
		CheckoutSessionID:    seed.Session.ID,
		ExternalCustomerID:   seed.ExternalCustomerID,
		TenantID:             confirmInputForSeed(seed).TenantID,
		ContextID:            confirmInputForSeed(seed).ContextID,
		EvaluationGeneration: stale.Session.EvaluationGeneration,
	})
	require.NoError(t, progressErr)
	require.Equal(t, model.CompletedCheckoutSessionStatus, progressed.Session.Status)
	require.NotNil(t, progressed.Session.ContractID)
	require.Equal(t, 1, h.fc.committedEntitlementCount())
	require.Equal(t, 2, h.countConfirmationSagasForSession(seed.Session.ID))

	current := h.querySession(seed.Session.ID)
	require.NotNil(t, current.DecisionID)
	require.NotEqual(t, decisionG1, *current.DecisionID)
	require.Equal(t, 1, h.countContractsForDecision(*current.DecisionID))
	require.Equal(t, 0, h.countContractsForDecision(decisionG1))
	h.driveSagaExpectAttemptFence(oldSaga.ID, current)
}

func TestE2E_ConfirmSaga_StaleWorkerRejectedBeforeSecondContractPrepare(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var firstPrepare atomic.Bool
	var prepareCommits atomic.Int32

	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforePrepareContract: func(context.Context) error {
				if !firstPrepare.CompareAndSwap(false, true) {
					return nil
				}

				close(entered)
				<-release

				return nil
			},
			AfterPrepareContract: func(context.Context) error {
				prepareCommits.Add(1)

				return nil
			},
		},
	})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)
	workerB := h.newWorkerExecutor("e2e-stale-prepare-worker-b")

	var confirmErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, confirmErr = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	}()

	select {
	case <-entered:
	case <-time.After(20 * time.Second):
		t.Fatal("worker A did not reach contract preparation")
	}

	claimed := h.querySession(seed.Session.ID)
	require.NotNil(t, claimed.ConfirmationAttemptID)
	attemptID := *claimed.ConfirmationAttemptID
	require.Nil(t, claimed.ContractID)
	sagaID := h.findSagaID(seed.Session.ID)

	h.advanceClock(h.SagaExecutor.LeaseDuration + time.Second)
	h.driveWithExecutor(workerB, sagaID)

	advanced := h.querySession(seed.Session.ID)
	require.Equal(t, attemptID, *advanced.ConfirmationAttemptID)
	require.NotNil(t, advanced.ContractID)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, advanced.Status)
	require.Equal(t, int32(1), prepareCommits.Load())
	require.Equal(t, 1, h.countContractsForDecision(*advanced.DecisionID))
	require.Equal(t, 1, h.countCheckoutPaymentAttempts(*advanced.ContractID))
	require.Equal(t, 1, h.fc.committedPaymentCount())

	close(release)
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("stale worker did not resume its prepare attempt")
	}

	require.NoError(t, confirmErr)
	require.Equal(t, int32(1), prepareCommits.Load(), "stale worker must not prepare a second contract")

	finalSession := h.querySession(seed.Session.ID)
	require.Equal(t, attemptID, *finalSession.ConfirmationAttemptID)
	require.Equal(t, *advanced.ContractID, *finalSession.ContractID)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, finalSession.Status)
	require.Equal(t, 1, h.countConfirmationSagasForSession(seed.Session.ID))
	require.Equal(t, 1, h.countContractsForDecision(*finalSession.DecisionID))
	require.Len(t, h.listCheckoutPaymentAttempts(*finalSession.ContractID), 1)
	require.Equal(t, 1, h.fc.committedPaymentCount())

	instance := h.loadSaga(sagaID)
	require.Equal(t, model.CompletedSagaStatus, instance.Status)
}

func TestE2E_ConfirmSaga_TransientFCFailureRetriesAfterDelay(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	h.fc.setEntitlementError(cerr.ServiceUnavailable(extcheckoutpayment.NewFailure(
		extcheckoutpayment.FailureClassTransient,
		errors.New("financial core unavailable"),
	)))
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	_, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.Error(t, err)

	sagaID := h.findSagaID(seed.Session.ID)
	instance := h.loadSaga(sagaID)
	require.Equal(t, model.RunningSagaStatus, instance.Status)
	require.NotNil(t, instance.NextAttemptAt)
	require.True(t, instance.NextAttemptAt.After(h.clock.Now()))
	require.NotNil(t, instance.LastError)
	require.Equal(t, 0, h.fc.committedEntitlementCount())

	restarted := h.newWorkerExecutor("e2e-transient-restart")
	require.NoError(t, restarted.ProcessDue(h.ctx))
	stillWaiting := h.loadSaga(sagaID)
	require.Equal(t, model.RunningSagaStatus, stillWaiting.Status)
	require.Equal(t, instance.Version, stillWaiting.Version)
	require.Equal(t, 0, h.fc.committedEntitlementCount())

	h.fc.setEntitlementSuccess()
	h.advanceClock(instance.NextAttemptAt.Sub(h.clock.Now()) + time.Second)
	h.driveWithExecutor(restarted, sagaID)

	session := h.querySession(seed.Session.ID)
	require.Equal(t, model.CompletedCheckoutSessionStatus, session.Status)
	require.Equal(t, 1, h.fc.committedEntitlementCount())
	require.Equal(t, 1, h.countContractsForDecision(*session.DecisionID))
}

func TestE2E_ConfirmSaga_PermanentFCConflictStopsRetry(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	h.fc.setEntitlementError(cerr.Conflict(extcheckoutpayment.NewFailure(
		extcheckoutpayment.FailureClassPermanentConflict,
		errors.New("stored receivable disagrees"),
	)))
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	_, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.Error(t, err)

	sagaID := h.findSagaID(seed.Session.ID)
	instance := h.loadSaga(sagaID)
	require.Equal(t, model.ReconciliationRequiredSagaStatus, instance.Status)
	require.NotNil(t, instance.LastError)
	require.Equal(t, 0, h.fc.committedEntitlementCount())
	callsAfterConflict := h.fc.callCount(fcCallRecordMerchantEntitlement, "")

	session := h.querySession(seed.Session.ID)
	require.NotNil(t, session.ContractID)
	contractRow := h.queryContractBySession(session)
	require.Nil(t, contractRow.FinancialMerchantEntitlementID)
	require.Nil(t, contractRow.FinancialReceivableID)

	_, replayErr := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.Error(t, replayErr)
	require.Equal(t, callsAfterConflict, h.fc.callCount(fcCallRecordMerchantEntitlement, ""))

	require.NoError(t, h.newWorkerExecutor("e2e-conflict-restart").ProcessDue(h.ctx))
	reloaded := h.loadSaga(sagaID)
	require.Equal(t, model.ReconciliationRequiredSagaStatus, reloaded.Status)
	require.Equal(t, instance.Version, reloaded.Version)
	require.Equal(t, callsAfterConflict, h.fc.callCount(fcCallRecordMerchantEntitlement, ""))

	unchanged := h.queryContractBySession(h.querySession(seed.Session.ID))
	require.Equal(t, contractRow.ID, unchanged.ID)
	require.Nil(t, unchanged.FinancialMerchantEntitlementID)
	require.Equal(t, model.PaymentPendingContractStatus, unchanged.Status)
}

func TestE2E_ConfirmSaga_HTTPCancellationLeavesSagaRecoverable(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	entered := make(chan struct{})
	var signaled atomic.Bool
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforeFCSubmit: func(context.Context) error {
				if signaled.CompareAndSwap(false, true) {
					close(entered)
				}

				return nil
			},
		},
	})
	h.fc.setEntitlementDelay(time.Hour)
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	requestCtx, cancel := context.WithCancel(h.ctx)
	done := make(chan struct{})
	var confirmErr error
	go func() {
		defer close(done)
		_, confirmErr = h.CheckoutSession.Confirm(requestCtx, confirmInputForSeed(seed))
	}()

	select {
	case <-entered:
	case <-time.After(20 * time.Second):
		t.Fatal("confirmation did not reach financial core submission")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("canceled Confirm did not return")
	}

	require.Error(t, confirmErr)
	require.Equal(t, 0, h.fc.committedEntitlementCount())

	sagaID := h.findSagaID(seed.Session.ID)
	instance := h.loadSaga(sagaID)
	require.Equal(t, model.RunningSagaStatus, instance.Status)
	require.NotEqual(t, model.CompensatedSagaStatus, instance.Status)
	require.NotNil(t, instance.NextAttemptAt)
	require.True(t, instance.NextAttemptAt.After(h.clock.Now()))

	session := h.querySession(seed.Session.ID)
	require.NotNil(t, session.ContractID)
	require.NotEqual(t, model.CompletedCheckoutSessionStatus, session.Status)
	require.Equal(t, model.PaymentPendingContractStatus, h.queryContractBySession(session).Status)

	h.fc.setEntitlementDelay(0)
	h.fc.setEntitlementSuccess()
	h.advanceClock(instance.NextAttemptAt.Sub(h.clock.Now()) + time.Second)
	h.driveWithExecutor(h.newWorkerExecutor("e2e-cancel-restart"), sagaID)

	completed := h.querySession(seed.Session.ID)
	require.Equal(t, model.CompletedCheckoutSessionStatus, completed.Status)
	require.Equal(t, 1, h.fc.committedEntitlementCount())
	require.Equal(t, *session.ContractID, *completed.ContractID)
}

func TestE2E_ConfirmSaga_AwaitingPaymentRestartsWithoutInventingSuccess(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)

	session := h.querySession(seed.Session.ID)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, session.Status)
	contractRow := h.queryContractBySession(session)
	require.Equal(t, model.PaymentPendingContractStatus, contractRow.Status)
	attempt := h.queryCheckoutPaymentAttempt(contractRow.ID)
	require.Equal(t, model.SubmittedPaymentAttemptState, attempt.State)
	require.NotNil(t, attempt.FinancialPaymentIntentID)
	intentID := *attempt.FinancialPaymentIntentID
	require.Equal(t, int64(0), h.creditExposureMinor(seed.CustomerID))

	sagaID := h.findSagaID(seed.Session.ID)
	require.Equal(t, model.CompletedSagaStatus, h.loadSaga(sagaID).Status)
	require.NoError(t, h.newWorkerExecutor("e2e-awaiting-result-restart").ProcessDue(h.ctx))
	require.Equal(t, model.ProcessingCheckoutSessionStatus, h.querySession(seed.Session.ID).Status)
	require.Equal(t, model.PaymentPendingContractStatus, h.queryContractBySession(session).Status)
	require.Equal(t, 1, h.fc.committedPaymentCount())

	mismatch := h.buildCheckoutPaymentProcessInput(attempt, contractRow, "fc-event-amount-mismatch")
	h.fc.setPaymentAuthoritativeSucceeded(intentID, attempt.AmountMinor+1, string(attempt.Currency), h.clock.Now().UTC())
	mismatch.PaidAmountMinor = attempt.AmountMinor + 1
	first := h.processPaymentResult(mismatch)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultReconciliationRequired, first.Result)

	reconciled := h.queryCheckoutPaymentAttempt(contractRow.ID)
	require.Equal(t, model.ReconciliationRequiredPaymentAttemptState, reconciled.State)
	require.NotNil(t, reconciled.FinancialPaymentIntentID)
	require.Equal(t, intentID, *reconciled.FinancialPaymentIntentID)
	require.Equal(t, model.PaymentPendingContractStatus, h.queryContractBySession(h.querySession(seed.Session.ID)).Status)
	require.Equal(t, int64(0), h.creditExposureMinor(seed.CustomerID))
	require.Equal(t, 0, h.countOutboxEvents("bnpl.checkout.completed.v1", seed.Session.ID.String()))

	inboxStatus, inboxCount := h.inboxStatus("financial-core", "fc-event-amount-mismatch")
	require.Equal(t, 1, inboxCount)
	require.Equal(t, "reconciliation_required", inboxStatus)

	repeat := h.buildCheckoutPaymentProcessInput(attempt, contractRow, "fc-event-amount-mismatch-repeat")
	h.fc.setPaymentAuthoritativeSucceeded(intentID, attempt.AmountMinor+1, string(attempt.Currency), h.clock.Now().UTC())
	repeat.PaidAmountMinor = attempt.AmountMinor + 1
	second := h.processPaymentResult(repeat)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultReconciliationRequired, second.Result)

	afterRepeat := h.queryCheckoutPaymentAttempt(contractRow.ID)
	require.Equal(t, model.ReconciliationRequiredPaymentAttemptState, afterRepeat.State)
	require.Equal(t, intentID, *afterRepeat.FinancialPaymentIntentID)
	require.Equal(t, model.PaymentPendingContractStatus, h.queryContractBySession(h.querySession(seed.Session.ID)).Status)
	require.Equal(t, int64(0), h.creditExposureMinor(seed.CustomerID))
	exposureCount, exposureAmount := h.countExposureIncreases(contractRow.ID)
	require.Equal(t, 0, exposureCount)
	require.Equal(t, int64(0), exposureAmount)
	require.Equal(t, 0, h.countOutboxEvents("bnpl.checkout.completed.v1", seed.Session.ID.String()))
	repeatStatus, repeatCount := h.inboxStatus("financial-core", "fc-event-amount-mismatch-repeat")
	require.Equal(t, 1, repeatCount)
	require.Equal(t, "reconciliation_required", repeatStatus)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var unpublished int
	require.NoError(t, testDB.QueryRow(ctx, `
		SELECT count(*)
		FROM bnpl.integration_outbox
		WHERE published_at IS NULL
		  AND message_key IN ($1, $2, $3)
	`, intentID, attempt.ID.String(), contractRow.ID.String()).Scan(&unpublished))
	require.GreaterOrEqual(t, unpublished, 1)
}
