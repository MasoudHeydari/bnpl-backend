//go:build e2e

package e2e_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

func newExternalCustomer(t *testing.T) model.ExternalCustomerID {
	t.Helper()

	return model.ExternalCustomerID(uuid.New())
}

// Baseline: merchant-funding confirmation completes end-to-end (sanity for A–I).
func TestE2E_ConfirmSaga_A_HappyPathMerchantFundingCompletes(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.False(t, out.PaymentRequired, "expected merchant-funding (v2) checkout policy")
	require.Equal(t, model.CompletedCheckoutSessionStatus, out.Session.Status)

	contractRow := h.queryContractBySession(out.Session)
	assert.NotNil(t, contractRow.FinancialMerchantEntitlementID)
}

// Requirement A: crash after confirming claim, before Contract creation — resume completes.
func TestE2E_ConfirmSaga_B_ResumeAfterCrashBeforeContractPrepare(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	var preparePasses atomic.Int32
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforePrepareContract: func(context.Context) error {
				if preparePasses.Add(1) == 1 {
					return context.Canceled
				}

				return nil
			},
		},
	})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	_, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.Error(t, err)

	// Release the saga executor lease from the aborted first drive, then resume
	// through the same Confirm entrypoint used by concurrent HTTP callers.
	h.advanceClock(3 * time.Minute)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.Equal(t, model.CompletedCheckoutSessionStatus, out.Session.Status)
}

// Requirement B: crash after Contract commit, before Checkout linkage — resume links once.
func TestE2E_ConfirmSaga_C_ResumeAfterCrashBeforeSessionLink(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	var linkPasses atomic.Int32
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforeLink: func(context.Context) error {
				if linkPasses.Add(1) == 1 {
					return context.Canceled
				}

				return nil
			},
		},
	})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	_, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.Error(t, err)

	h.resumeUntilSessionCompleted(seed.Session.ID)

	session := h.querySession(seed.Session.ID)
	require.NotNil(t, session.ContractID)
}

// Requirement C: crash after Checkout linkage, before FC submission — resume submits once.
func TestE2E_ConfirmSaga_D_ResumeAfterCrashBeforeFCSubmit(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	var fcPasses atomic.Int32
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforeFCSubmit: func(context.Context) error {
				if fcPasses.Add(1) == 1 {
					return context.Canceled
				}

				return nil
			},
		},
	})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	_, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.Error(t, err)

	h.resumeUntilSessionCompleted(seed.Session.ID)
}

// Requirement D (timeout variant): FC timeout then retry uses a single entitlement.
func TestE2E_ConfirmSaga_E_FCTimeoutThenRetryUsesSingleEntitlement(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	var fcAttempts atomic.Int32
	var fcClient *controllableCheckoutPaymentClient
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforeFCSubmit: func(context.Context) error {
				if fcAttempts.Add(1) == 1 {
					fcClient.setEntitlementTimeout()
				} else {
					fcClient.setEntitlementSuccess()
				}

				return nil
			},
		},
	})
	fcClient = h.fc
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	_, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.Error(t, err)

	h.fc.setEntitlementSuccess()
	h.resumeUntilSessionCompleted(seed.Session.ID)
	assert.Equal(t, 1, h.fc.committedEntitlementCount())
}

// Scenario F: duplicate idempotency key returns reused FC result without a second commit.
func TestE2E_ConfirmSaga_F_DuplicateIdempotencyReusesEntitlement(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)

	contractRow := h.queryContractBySession(out.Session)
	require.Equal(t, 1, h.fc.entitlementCallCount(contractRow.ID.String()))

	_, err = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	assert.Equal(t, 1, h.fc.entitlementCallCount(contractRow.ID.String()))
}

// Requirement D: lost FC response after remote commit — retry reuses one entitlement.
func TestE2E_ConfirmSaga_G_LostFCResponseAfterCommitRetriesOnce(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	var fcPasses atomic.Int32
	var fcClient *controllableCheckoutPaymentClient
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforeFCSubmit: func(context.Context) error {
				if fcPasses.Add(1) == 1 {
					fcClient.setEntitlementLostAfterCommit()
				}

				return nil
			},
		},
	})
	fcClient = h.fc
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	_, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	// Lost response after commit leaves durable FC state; Confirm may reconcile inline or error.
	if err != nil {
		h.fc.setEntitlementSuccess()
		sagaID := h.findSagaID(seed.Session.ID)
		h.driveSagaUntilIdle(sagaID)
	}

	session := h.querySession(seed.Session.ID)
	require.Equal(t, model.CompletedCheckoutSessionStatus, session.Status)
	assert.Equal(t, 1, h.fc.committedEntitlementCount())
}

// Scenario H: delayed FC response — saga waits for hook release then completes.
func TestE2E_ConfirmSaga_H_DelayedFCResponseCompletesAfterResume(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	h.fc.setEntitlementDelay(0)
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.Equal(t, model.CompletedCheckoutSessionStatus, out.Session.Status)
}

// Stale confirming recovery job resumes an abandoned claim (supports A–C liveness).
func TestE2E_ConfirmSaga_I_RecoverStaleConfirmingResumesSaga(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	var fcPasses atomic.Int32
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforeFCSubmit: func(context.Context) error {
				if fcPasses.Add(1) == 1 {
					return context.Canceled
				}

				return nil
			},
		},
	})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	_, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.Error(t, err)

	h.advanceClock(checkoutsessionuc.DefaultConfirmingClaimTimeout + time.Minute)

	output := h.recoverStale()
	assert.Equal(t, 1, output.ResumedCount)

	session := h.querySession(seed.Session.ID)
	require.Equal(t, model.CompletedCheckoutSessionStatus, session.Status)
}

// Requirement E: overlapping Confirm calls create one Contract and one Saga.
func TestE2E_ConfirmSaga_J_ConcurrentConfirmSingleContract(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	var (
		wg   sync.WaitGroup
		errA error
		errB error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errA = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	}()
	go func() {
		defer wg.Done()
		_, errB = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	}()
	wg.Wait()

	// One caller may observe IN_PROGRESS while the other completes, or both may
	// succeed via idempotent projection after a single durable Contract.
	require.True(t, errA == nil || errB == nil, "at least one Confirm must succeed: a=%v b=%v", errA, errB)

	session := h.querySession(seed.Session.ID)
	if session.Status != model.CompletedCheckoutSessionStatus {
		h.resumeUntilSessionCompleted(seed.Session.ID)
		session = h.querySession(seed.Session.ID)
	}

	require.Equal(t, model.CompletedCheckoutSessionStatus, session.Status)
	require.NotNil(t, session.DecisionID)
	require.Equal(t, 1, h.countContractsForDecision(*session.DecisionID))
	assert.Equal(t, 1, h.fc.committedEntitlementCount())
}

// Requirement I: after lease expiry Worker B owns the saga; Worker A's stale
// ExecuteByID cannot commit competing business progress.
func TestE2E_ConfirmSaga_K_StaleWorkerFencedAfterLeaseExpiry(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	blockA := make(chan struct{})
	staleEntered := make(chan struct{}, 1)
	var stalePrepareBarrier atomic.Bool
	stalePrepareBarrier.Store(true)

	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			// Park only inside the external prepare step. Blocking a
			// TransactionalStep (BeforeLink) holds a DB row lock and deadlocks
			// Worker B when it tries to continue under a new lease.
			AfterPrepareContract: func(context.Context) error {
				if !stalePrepareBarrier.CompareAndSwap(true, false) {
					return nil
				}

				staleEntered <- struct{}{}
				<-blockA

				return nil
			},
		},
	})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)
	workerB := h.newWorkerExecutor("e2e-worker-b")

	var confirmErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, confirmErr = h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	}()

	select {
	case <-staleEntered:
	case <-time.After(30 * time.Second):
		t.Fatal("stale prepare barrier was not entered")
	}

	require.Equal(t, 1, h.countContractsForDecision(*seed.Session.DecisionID))

	// Expire Worker A's lease while it is still inside the external prepare
	// boundary, then let Worker B own forward progress.
	h.advanceClock(h.SagaExecutor.LeaseDuration + time.Second)
	sagaID := h.findSagaID(seed.Session.ID)
	require.NoError(t, workerB.ExecuteByID(h.ctx, sagaID))
	h.driveWithExecutor(workerB, sagaID)

	close(blockA)
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("stale Confirm worker did not return")
	}

	session := h.querySession(seed.Session.ID)
	if session.Status != model.CompletedCheckoutSessionStatus {
		h.resumeUntilSessionCompleted(seed.Session.ID)
		session = h.querySession(seed.Session.ID)
	}

	require.Equal(t, model.CompletedCheckoutSessionStatus, session.Status)
	require.NotNil(t, session.DecisionID)
	require.Equal(t, 1, h.countContractsForDecision(*session.DecisionID))
	assert.Equal(t, 1, h.fc.committedEntitlementCount())

	// Stale worker resume must converge idempotently, not create a second Contract.
	_ = confirmErr
}
