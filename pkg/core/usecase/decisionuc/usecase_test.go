package decisionuc

import (
	"testing"

	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
)

func testCheckoutRegistry(t *testing.T) *checkout.Registry {
	t.Helper()

	registry, err := checkoutcatalog.NewRegistry()
	require.NoError(t, err)

	return registry
}

func TestNew_ConstructsWithValidDependencies(t *testing.T) {
	t.Parallel()

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
		testCheckoutRegistry(t),
		checkoutcatalog.DefaultRef(),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
		mustTestJurisdictions(t),
		metricsnoop.New(),
		AllowAllNewCreditUsage{},
		newInternalTestObserver(t),
	)
	require.NoError(t, err)
	require.NotNil(t, uc)
}

func TestNew_RequiresCreditAccount(t *testing.T) {
	t.Parallel()

	_, err := New(
		mock.NewPool(t),
		mock.NewDecision(t),
		mock.NewCustomerProfile(t),
		nil,
		mock.NewMerchantProfile(t),
		mock.NewMerchantBranchProfile(t),
		mock.NewCheckoutDecisionAllocation(t),
		mock.NewPolicyAssignment(t),
		mock.NewInstallment(t),
		mock.NewInstallmentCharge(t),
		mock.NewOutbox(t),
		mock.NewClock(t),
		sha256fingerprint.New(),
		testCheckoutRegistry(t),
		checkoutcatalog.DefaultRef(),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
		mustTestJurisdictions(t),
		metricsnoop.New(),
		AllowAllNewCreditUsage{},
		newInternalTestObserver(t),
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "decision use case dependencies are required")
}

func TestNew_RequiresCheckoutPolicy(t *testing.T) {
	t.Parallel()

	_, err := New(
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
		nil,
		checkoutcatalog.DefaultRef(),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
		mustTestJurisdictions(t),
		metricsnoop.New(),
		AllowAllNewCreditUsage{},
		newInternalTestObserver(t),
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "decision use case dependencies are required")
}

func TestNew_RequiresJurisdictionCapability(t *testing.T) {
	t.Parallel()

	_, err := New(
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
		testCheckoutRegistry(t),
		checkoutcatalog.DefaultRef(),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
		nil,
		metricsnoop.New(),
		AllowAllNewCreditUsage{},
		newInternalTestObserver(t),
	)
	require.Error(t, err)
	require.ErrorContains(t, err, "jurisdiction capability is required")
}

func mustTestJurisdictions(t *testing.T) *domaincapability.Jurisdictions {
	t.Helper()

	caps, err := domaincapability.NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction})
	require.NoError(t, err)

	return caps
}

// newInternalTestObserver returns a DecisionObserver for white-box tests.
func newInternalTestObserver(t *testing.T) *mock.DecisionObserver {
	t.Helper()

	observer := mock.NewDecisionObserver(t)
	observer.EXPECT().EvaluationStarted(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().AvailableCreditChecked(
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Maybe()
	observer.EXPECT().DecisionApproved(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().DecisionPartiallyApproved(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().DecisionDenied(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().DecisionPersisted(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().DecisionReplayed(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().OfferExpired(testifymock.Anything, testifymock.Anything).Maybe()
	observer.EXPECT().OfferInvalidated(testifymock.Anything, testifymock.Anything, testifymock.Anything).Maybe()

	return observer
}
