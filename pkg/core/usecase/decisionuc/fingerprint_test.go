package decisionuc

import (
	"testing"

	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
)

func TestCheckoutFingerprint_DeterministicAndSensitiveToInputs(t *testing.T) {
	t.Parallel()

	registry, err := checkoutcatalog.NewRegistry()
	require.NoError(t, err)

	uc, err := New(
		mock.NewPool(t),
		mock.NewDecision(t),
		mock.NewCustomerProfile(t),
		mock.NewCreditAccount(t),
		mock.NewMerchantProfile(t),
		mock.NewMerchantBranchProfile(t),
		mock.NewCheckoutDecisionAllocation(t),
		mock.NewPolicyAssignment(t),
		mock.NewInstallment(t),
		mock.NewInstallmentCharge(t),
		mock.NewOutbox(t),
		mock.NewClock(t),
		sha256fingerprint.New(),
		registry,
		checkoutcatalog.DefaultRef(),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
		mustTestJurisdictions(t),
		metricsnoop.New(),
		AllowAllNewCreditUsage{},
	)
	require.NoError(t, err)

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	purchaseContext := PurchaseContext{
		Channel:  model.POSPurchaseChannel,
		PosGpsID: model.PosGpsID(uuid.New()),
	}

	first, err := uc.checkoutFingerprint(
		"checkout-intent-1",
		externalCustomerID,
		purchaseContext,
		100_000,
		"IRR",
		"checkout_contract_approval",
		"v1",
	)
	require.NoError(t, err)

	second, err := uc.checkoutFingerprint(
		"checkout-intent-1",
		externalCustomerID,
		purchaseContext,
		100_000,
		"IRR",
		"checkout_contract_approval",
		"v1",
	)
	require.NoError(t, err)
	require.Equal(t, first, second)

	differentAmount, err := uc.checkoutFingerprint(
		"checkout-intent-1",
		externalCustomerID,
		purchaseContext,
		100_001,
		"IRR",
		"checkout_contract_approval",
		"v1",
	)
	require.NoError(t, err)
	require.NotEqual(t, first, differentAmount)

	differentSource, err := uc.checkoutFingerprint(
		"checkout-intent-1",
		externalCustomerID,
		PurchaseContext{
			Channel:  model.POSPurchaseChannel,
			PosGpsID: model.PosGpsID(uuid.New()),
		},
		100_000,
		"IRR",
		"checkout_contract_approval",
		"v1",
	)
	require.NoError(t, err)
	require.NotEqual(t, first, differentSource)
}
