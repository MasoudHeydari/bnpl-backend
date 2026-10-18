//go:build e2e

package e2e_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
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

	input := h.buildCheckoutPaymentProcessInput(attempt, contractRow, "fc-event-dup-success")
	h.processPaymentResult(input)
	h.processPaymentResult(input)

	session := h.querySession(seed.Session.ID)
	require.Equal(t, model.CompletedCheckoutSessionStatus, session.Status)
	require.Equal(t, 1, h.countCheckoutPaymentAttempts(contractRow.ID))
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
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)

	contractRow := h.queryContractBySession(out.Session)
	attempt := h.queryCheckoutPaymentAttempt(contractRow.ID)

	failure := h.buildCheckoutPaymentFailureProcessInput(attempt, contractRow, "fc-event-terminal-failure")
	h.processPaymentResult(failure)

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

	assert.LessOrEqual(t, h.countCheckoutPaymentAttempts(contractRow.ID), 2)
}
