//go:build e2e

package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

const (
	confirmSagaTestAmountMinor = uint64(10_000_000)
	confirmSagaReturnDest      = "e2e-return"
)

type confirmSagaSeed struct {
	MerchantID         model.MerchantID
	OnlineStoreID      model.OnlineStoreID
	ExternalCustomerID model.ExternalCustomerID
	CustomerID         model.CustomerID
	Session            model.CheckoutSession
}

func seedMerchantFundingCheckout(
	t *testing.T,
	h *confirmSagaHarness,
	externalCustomerID model.ExternalCustomerID,
) confirmSagaSeed {
	t.Helper()

	customerID := insertCustomerProfile(t, externalCustomerID, model.ActiveCustomerStatus)
	insertCreditAccount(t, customerID, 90_000_000)
	insertCheckoutPolicyV2Assignment(t, externalCustomerID)

	merchantID := model.MerchantID(uuid.New())
	onlineStoreID := model.OnlineStoreID(uuid.New())
	merchantProfileID, branchProfileID := insertMerchantOnlineStack(
		t,
		merchantID,
		onlineStoreID,
		confirmSagaReturnDest,
	)

	_ = merchantProfileID
	_ = branchProfileID

	out, err := h.CheckoutSession.CreatePartner(h.ctx, checkoutsessionuc.CreatePartnerInput{
		CheckoutSessionRequest: checkoutsessionuc.CheckoutSessionRequest{
			MerchantID:         merchantID,
			OnlineStoreID:      onlineStoreID,
			MerchantOrderID:    "order-" + uuid.NewString(),
			IdempotencyKey:     "idem-" + uuid.NewString(),
			CustomerReference:  "cust-ref",
			ReturnDestination:  confirmSagaReturnDest,
			MerchantState:      "state",
			RequestedAmountMinor: confirmSagaTestAmountMinor,
			Currency:           model.IRRCurrency,
		},
		CustomerPhone: "+989121111111",
	})
	require.NoError(t, err)
	require.NotNil(t, out.Offer)
	require.Equal(t, checkoutsessionuc.ApprovedOfferOutcome, out.Offer.Outcome)

	session := h.querySession(out.Session.ID)
	require.NotNil(t, session.DecisionID)

	decision := queryDecisionByID(t, *session.DecisionID)
	require.Equal(t, "checkout_contract_approval", decision.PolicyID)
	require.Equal(t, "v2", decision.PolicyVersion, "policy assignment must select merchant-funding checkout policy")

	return confirmSagaSeed{
		MerchantID:         merchantID,
		OnlineStoreID:      onlineStoreID,
		ExternalCustomerID: externalCustomerID,
		CustomerID:         customerID,
		Session:            session,
	}
}

func insertCreditAccount(t *testing.T, customerID model.CustomerID, limitMinor uint64) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := testDB.Exec(ctx, `
		INSERT INTO bnpl.credit_accounts (
			customer_id, limit_amount_minor, exposure_amount_minor, currency,
			status, effective_from, created_at, updated_at
		) VALUES ($1, $2, 0, 'IRR', 'active', now(), now(), now())
	`, customerID, limitMinor)
	require.NoError(t, err)
}

func insertCheckoutPolicyV2Assignment(t *testing.T, externalCustomerID model.ExternalCustomerID) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := testDB.Exec(ctx, `
		INSERT INTO bnpl.policy_assignments (
			id, subject_type, subject_id, policy_family, policy_id, policy_version,
			reason, source, valid_from, created_by, created_at, updated_at
		) VALUES (
			$1, 'customer', $2, 'checkout', 'checkout_contract_approval', 'v2',
			'e2e_trusted', 'manual', now() - interval '1 day', 'e2e', now(), now()
		)
	`, uuid.New(), externalCustomerID.String())
	require.NoError(t, err)
}

func insertMerchantOnlineStack(
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
			$1, 'e2e-merchant', 'active', 'low', 12, 'IR',
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
			'["platform_funded"]', '["direct"]', '["IRR"]', 'on_create'
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

func confirmInputForSeed(seed confirmSagaSeed) checkoutsessionuc.ConfirmInput {
	return checkoutsessionuc.ConfirmInput{
		CheckoutSessionID:    seed.Session.ID,
		ExternalCustomerID:   seed.ExternalCustomerID,
		TenantID:             model.TenantID(uuid.New()),
		ContextID:            model.ContextID(uuid.New()),
		EvaluationGeneration: seed.Session.EvaluationGeneration,
	}
}
