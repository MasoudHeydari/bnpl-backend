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

func TestE2E_ConfirmSaga_L_FirstConfirmMintsAttemptAndSagaBusinessKey(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.Equal(t, model.CompletedCheckoutSessionStatus, out.Session.Status)

	session := h.querySession(seed.Session.ID)
	h.assertAttemptMatchesSaga(session)
	require.Equal(t, 1, h.countConfirmationSagasForSession(session.ID))
}

func TestE2E_ConfirmSaga_M_ConcurrentConfirmReusesSameAttempt(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	var (
		wg    sync.WaitGroup
		errA  error
		errB  error
		outA  checkoutsessionuc.ConfirmOutput
		outB  checkoutsessionuc.ConfirmOutput
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		outA, errA = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	}()
	go func() {
		defer wg.Done()
		outB, errB = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	}()
	wg.Wait()

	require.True(t, errA == nil || errB == nil, "at least one Confirm must succeed: a=%v b=%v", errA, errB)

	session := h.querySession(seed.Session.ID)
	if session.Status != model.CompletedCheckoutSessionStatus {
		h.resumeUntilSessionCompleted(seed.Session.ID)
		session = h.querySession(seed.Session.ID)
	}

	require.Equal(t, model.CompletedCheckoutSessionStatus, session.Status)
	require.Equal(t, 1, h.countConfirmationSagasForSession(session.ID))
	require.NotNil(t, session.ConfirmationAttemptID)

	if errA == nil && outA.Session.ConfirmationAttemptID != nil {
		require.Equal(t, *session.ConfirmationAttemptID, *outA.Session.ConfirmationAttemptID)
	}

	if errB == nil && outB.Session.ConfirmationAttemptID != nil {
		require.Equal(t, *session.ConfirmationAttemptID, *outB.Session.ConfirmationAttemptID)
	}
}

func TestE2E_ConfirmSaga_N_NewConfirmAfterCompensateUsesNewAttemptAndFencesOldSaga(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)

	var (
		customerID    model.CustomerID
		preparePasses atomic.Int32
	)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforePrepareContract: func(context.Context) error {
				// Zero credit only after the confirmation attempt/Saga exist so
				// compensation (not pre-claim revalidation) owns the reopen.
				if preparePasses.Add(1) == 1 && !customerID.IsZero() {
					updateCreditAccountLimit(t, customerID, 0)
				}

				return nil
			},
		},
	})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)
	customerID = seed.CustomerID

	firstOut, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	if err == nil {
		require.True(t, firstOut.ReconfirmRequired, "insufficient credit must compensate and require reconfirm")
	}

	firstAttemptSession := h.querySession(seed.Session.ID)
	if firstAttemptSession.Status != model.OpenCheckoutSessionStatus {
		require.Equal(t, 1, h.countConfirmationSagasForSession(seed.Session.ID),
			"confirming without a saga after prepare rejection")
		compensatingSaga := h.oldestConfirmationSagaForSession(seed.Session.ID)
		t.Logf("after Confirm: session=%s saga status=%s phase=%s step=%s reason=%v confirmErr=%v reconfirm=%v",
			firstAttemptSession.Status,
			compensatingSaga.Status,
			compensatingSaga.Phase,
			compensatingSaga.CurrentStep,
			compensatingSaga.LastError,
			err,
			firstOut.ReconfirmRequired,
		)
		h.driveSagaUntilIdle(compensatingSaga.ID)
		firstAttemptSession = h.querySession(seed.Session.ID)
	}

	require.Equal(t, model.OpenCheckoutSessionStatus, firstAttemptSession.Status,
		"compensated confirmation must reopen checkout for a later attempt")
	require.True(t, firstAttemptSession.ConfirmationAttemptID == nil || firstAttemptSession.ConfirmationAttemptID.IsZero())

	oldInstance := h.oldestConfirmationSagaForSession(seed.Session.ID)
	oldSagaID := oldInstance.ID

	updateCreditAccountLimit(t, seed.CustomerID, 90_000_000)

	started, err := h.CheckoutSession.Start(h.ctx, checkoutsessionuc.StartInput{
		CheckoutSessionID:  seed.Session.ID,
		ExternalCustomerID: seed.ExternalCustomerID,
	})
	require.NoError(t, err)
	require.Equal(t, checkoutsessionuc.ApprovedOfferOutcome, started.Offer.Outcome)

	out, err := h.CheckoutSession.Confirm(h.ctx, checkoutsessionuc.ConfirmInput{
		CheckoutSessionID:    seed.Session.ID,
		ExternalCustomerID:   seed.ExternalCustomerID,
		TenantID:             confirmInputForSeed(seed).TenantID,
		ContextID:            confirmInputForSeed(seed).ContextID,
		EvaluationGeneration: started.Session.EvaluationGeneration,
	})
	require.NoError(t, err)
	require.Equal(t, model.CompletedCheckoutSessionStatus, out.Session.Status)

	newSession := h.querySession(seed.Session.ID)
	require.NotNil(t, newSession.ConfirmationAttemptID)
	h.assertAttemptMatchesSaga(newSession)
	require.NotEqual(t, string(oldInstance.BusinessKey), newSession.ConfirmationAttemptID.String())
	require.Equal(t, 2, h.countConfirmationSagasForSession(seed.Session.ID))

	h.driveSagaExpectAttemptFence(oldSagaID, newSession)
}

func TestE2E_ConfirmSaga_O_CompletedReplayIsIdempotent(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	first, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.Equal(t, model.CompletedCheckoutSessionStatus, first.Session.Status)

	second, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.Equal(t, model.CompletedCheckoutSessionStatus, second.Session.Status)
	require.Equal(t, first.Session.ContractID, second.Session.ContractID)
	require.Equal(t, 1, h.countContractsForDecision(*first.Session.DecisionID))
	assert.Equal(t, 1, h.fc.committedEntitlementCount())
}

func TestE2E_ConfirmSaga_P_StaleEvaluationGenerationDoesNotConfirm(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	currentGen := seed.Session.EvaluationGeneration
	require.GreaterOrEqual(t, currentGen, 1)

	// Advance durable generation (and clear Decision like production bump) so a
	// still-valid positive stale generation can be presented.
	bumpCheckoutEvaluationGeneration(t, seed.Session.ID)

	stale := confirmInputForSeed(seed)
	stale.EvaluationGeneration = currentGen

	out, err := h.CheckoutSession.Confirm(h.ctx, stale)
	require.NoError(t, err)
	require.True(t, out.ReconfirmRequired)
	require.Equal(t, 0, h.countContractsForDecision(*seed.Session.DecisionID))

	session := h.querySession(seed.Session.ID)
	require.Equal(t, model.OpenCheckoutSessionStatus, session.Status)
	require.Nil(t, session.ContractID)
	require.Greater(t, session.EvaluationGeneration, currentGen)
}
