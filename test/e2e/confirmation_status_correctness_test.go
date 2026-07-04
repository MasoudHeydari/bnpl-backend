//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/checkoutsessionrs"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

func TestE2E_ConfirmationStatus_ReconfirmAfterCompensateBeforeOfferRefresh(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	var customerID model.CustomerID
	var starveOnce atomic.Bool
	starveOnce.Store(true)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{
		externalCustomerID: externalCustomerID,
		hooks: checkoutsessionuc.ConfirmationSagaHooks{
			BeforePrepareContract: func(context.Context) error {
				if starveOnce.CompareAndSwap(true, false) && !customerID.IsZero() {
					updateCreditAccountLimit(t, customerID, 0)
				}

				return nil
			},
		},
	})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)
	customerID = seed.CustomerID
	generationG1 := seed.Session.EvaluationGeneration
	decisionG1 := *seed.Session.DecisionID

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.True(t, out.ReconfirmRequired)

	compensated := h.querySession(seed.Session.ID)
	require.Equal(t, model.OpenCheckoutSessionStatus, compensated.Status)
	require.Nil(t, compensated.ContractID)
	require.True(t, compensated.ConfirmationAttemptID == nil || compensated.ConfirmationAttemptID.IsZero())
	require.Nil(t, compensated.DecisionID)
	require.Greater(t, compensated.EvaluationGeneration, generationG1)
	require.Equal(t, 0, h.countContractsForDecision(decisionG1))

	// Interrupted before Start/offer refresh: status must still project reconfirm.
	got, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, model.OpenCheckoutSessionStatus, got.Status)
	require.Equal(t, checkoutsessionuc.ConfirmationStatusReconfirmRequired, got.ConfirmationStatus)
	require.Equal(t, checkoutsessionuc.ConfirmationNextActionReconfirm, got.NextAction)

	updateCreditAccountLimit(t, seed.CustomerID, 90_000_000)

	stale, staleErr := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, staleErr)
	require.True(t, stale.ReconfirmRequired)
	require.Equal(t, 0, h.countContractsForDecision(decisionG1))

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
	require.Equal(t, 1, h.countContractsForDecision(*progressed.Session.DecisionID))
	require.NotEqual(t, decisionG1, *progressed.Session.DecisionID)
}

func TestE2E_ConfirmationStatus_FirstTimeOnStartWithoutDecision(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})

	customerID := insertCustomerProfile(t, externalCustomerID, model.ActiveCustomerStatus)
	insertCreditAccount(t, customerID, 90_000_000)
	insertCheckoutPolicyV2Assignment(t, externalCustomerID)

	merchantID := model.MerchantID(uuid.New())
	onlineStoreID := model.OnlineStoreID(uuid.New())
	_, branchProfileID := insertMerchantOnlineStackOnStart(t, merchantID, onlineStoreID, confirmSagaReturnDest)
	_ = branchProfileID

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
	require.Nil(t, out.Offer)

	session := h.querySession(out.Session.ID)
	require.Equal(t, 1, session.EvaluationGeneration)
	require.Nil(t, session.DecisionID)
	require.Nil(t, session.ContractID)

	got, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, checkoutsessionuc.ConfirmationStatusNotStarted, got.ConfirmationStatus)
	require.Equal(t, checkoutsessionuc.ConfirmationNextActionStartConfirm, got.NextAction)
	require.NotEqual(t, checkoutsessionuc.ConfirmationStatusReconfirmRequired, got.ConfirmationStatus)
}

func TestE2E_ConfirmationStatus_HTTPBoundary(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	checkoutsessionrs.RegisterCustomerToken(engine.Group("/api/v1"), h.CheckoutSession, checkoutsessionrs.RegisterOptions{})

	beforeFC := len(h.fc.callRecords())
	beforeOutbox := h.countIntegrationOutboxRows()

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/checkout-sessions/"+seed.Session.PublicToken.String()+"/confirmation-status",
		nil,
	)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	var envelope map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	data, ok := envelope["data"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, seed.Session.PublicToken.String(), data["checkout_session_id"])
	require.Equal(t, "open", data["status"])
	require.Equal(t, "not_started", data["confirmation_status"])
	require.Equal(t, "start_confirm", data["next_action"])

	body := strings.ToLower(rec.Body.String())
	for _, forbidden := range []string{
		"payment_url",
		"\"contract_id\"",
		"saga_id",
		"confirmation_attempt",
		"financial_payment",
		"\"otp\"",
		"phone",
		"\"customer_id\"",
	} {
		require.NotContains(t, body, forbidden)
	}

	require.Equal(t, beforeFC, len(h.fc.callRecords()))
	require.Equal(t, beforeOutbox, h.countIntegrationOutboxRows())
}

func TestE2E_ConfirmationStatus_PaymentSucceededButCheckoutStillProcessing(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedInitialPaymentCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.True(t, out.PaymentRequired)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, out.Session.Status)

	attempt := h.queryCheckoutPaymentAttempt(*out.Session.ContractID)
	_, err = testDB.Exec(h.ctx, `
		UPDATE bnpl.payment_attempts
		SET state = $2,
			financial_paid_at = COALESCE(financial_paid_at, now()),
			applied_at = COALESCE(applied_at, now()),
			updated_at = now()
		WHERE id = $1
	`, attempt.ID, model.SucceededPaymentAttemptState)
	require.NoError(t, err)

	session := h.querySession(seed.Session.ID)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, session.Status)

	got, err := h.CheckoutSession.GetExternalConfirmationStatus(h.ctx, checkoutsessionuc.GetExternalConfirmationStatusInput{
		Token: seed.Session.PublicToken,
	})
	require.NoError(t, err)
	require.Equal(t, model.ProcessingCheckoutSessionStatus, got.Status)
	require.NotEqual(t, checkoutsessionuc.ConfirmationStatusCompleted, got.ConfirmationStatus)
	require.NotEqual(t, checkoutsessionuc.ConfirmationNextActionShowSuccess, got.NextAction)
	require.Equal(t, checkoutsessionuc.ConfirmationStatusPaymentProcessing, got.ConfirmationStatus)
}

func TestE2E_ConfirmSaga_RetryBudgetAllowsNineExecutions(t *testing.T) {
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
	require.Equal(t, 1, instance.StepAttemptCount)

	worker := h.newWorkerExecutor("e2e-retry-budget")
	for step := 0; step < 16; step++ {
		instance = h.loadSaga(sagaID)
		if instance.Status == model.ManualReviewRequiredSagaStatus {
			break
		}

		require.Contains(t, []model.SagaStatus{
			model.RunningSagaStatus,
			model.WaitingSagaStatus,
		}, instance.Status)
		require.LessOrEqual(t, instance.StepAttemptCount, 9)

		if instance.NextAttemptAt != nil && instance.NextAttemptAt.After(h.clock.Now()) {
			h.advanceClock(instance.NextAttemptAt.Sub(h.clock.Now()) + time.Second)
		}

		err := worker.ExecuteByID(h.ctx, sagaID)
		if err != nil {
			require.ErrorIs(t, err, repo.ErrSagaNotRunnable)
		}
	}

	instance = h.loadSaga(sagaID)
	require.Equal(t, model.ManualReviewRequiredSagaStatus, instance.Status)
	require.Equal(t, 9, instance.StepAttemptCount)
	require.Equal(t, 9, h.fc.callCount(fcCallRecordMerchantEntitlement, ""))
	require.Equal(t, 0, h.fc.committedEntitlementCount())
	require.Equal(t, 1, h.countContractsForDecision(*h.querySession(seed.Session.ID).DecisionID))

	version := instance.Version
	require.NoError(t, worker.ProcessDue(h.ctx))
	err = worker.ExecuteByID(h.ctx, sagaID)
	require.ErrorIs(t, err, repo.ErrSagaNotRunnable)
	final := h.loadSaga(sagaID)
	require.Equal(t, model.ManualReviewRequiredSagaStatus, final.Status)
	require.Equal(t, 9, final.StepAttemptCount)
	require.Equal(t, version, final.Version)
	require.Equal(t, 9, h.fc.callCount(fcCallRecordMerchantEntitlement, ""))
}

func insertMerchantOnlineStackOnStart(
	t *testing.T,
	merchantID model.MerchantID,
	onlineStoreID model.OnlineStoreID,
	returnDestination string,
) (model.MerchantProfileID, model.MerchantBranchProfileID) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var merchantProfileIDStr string
	require.NoError(t, testDB.QueryRow(ctx, `
		INSERT INTO bnpl.merchant_profiles (
			merchant_id, display_name, status, risk_tier, max_tenure_months, jurisdiction,
			funding_models, settlement_models, currencies, max_amount_minor, max_amount_currency
		) VALUES (
			$1, 'e2e-merchant-on-start', 'active', 'low', 12, 'IR',
			'["platform_funded"]', '["direct"]', '["IRR"]', 2000000000, 'IRR'
		) RETURNING id
	`, merchantID).Scan(&merchantProfileIDStr))
	merchantProfileID := model.MerchantProfileID(uuid.MustParse(merchantProfileIDStr))

	var branchProfileIDStr string
	require.NoError(t, testDB.QueryRow(ctx, `
		INSERT INTO bnpl.merchant_branch_profiles (
			merchant_id, branch_id, branch_type, status, jurisdiction,
			funding_models, settlement_models, currencies, offer_preparation_mode
		) VALUES (
			$1, $2, 'online', 'active', 'IR',
			'["platform_funded"]', '["direct"]', '["IRR"]', 'on_start'
		) RETURNING id
	`, merchantID, uuid.New()).Scan(&branchProfileIDStr))
	branchProfileID := model.MerchantBranchProfileID(uuid.MustParse(branchProfileIDStr))

	_, err := testDB.Exec(ctx, `
		INSERT INTO bnpl.merchant_online_stores (
			merchant_branch_profile_id, online_store_id, enabled, created_at, updated_at
		) VALUES ($1, $2, true, now(), now())
	`, branchProfileID, onlineStoreID)
	require.NoError(t, err)

	_, err = testDB.Exec(ctx, `
		INSERT INTO bnpl.merchant_return_destinations (
			merchant_id, destination_name, return_url, enabled, created_at, updated_at
		) VALUES ($1, $2, 'https://merchant.example/return', true, now(), now())
	`, merchantID, returnDestination)
	require.NoError(t, err)

	return merchantProfileID, branchProfileID
}
