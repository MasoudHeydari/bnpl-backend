package config

import (
	"testing"
	"time"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	initialcreditcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/catalog"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

func TestMigrations_newUseCase(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	migrations := mock.NewMigrations(t)
	clock := mock.NewClock(t)
	logger := mock.NewLogger(t)

	m := Migrations{FilesDir: "/var/migrations"}

	uc, err := m.newUseCase(pool, migrations, clock, logger)
	require.NoError(t, err)
	require.NotNil(t, uc)
}

func TestOutboxPublisher_newUseCase(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	outbox := mock.NewOutbox(t)
	publisher := mock.NewPublisher(t)
	clock := mock.NewClock(t)
	logger := mock.NewLogger(t)

	obp := OutboxPublisher{
		WorkerID:       "worker-1",
		BatchSize:      25,
		RetryBaseDelay: 2 * time.Second,
		RetryMaxDelay:  30 * time.Second,
	}

	uc := obp.newUseCase(
		pool,
		outbox,
		publisher,
		clock,
		logger,
	)
	require.NotNil(t, uc)
	assert.Equal(t, "worker-1", uc.WorkerID)
	assert.Equal(t, 25, uc.BatchSize)
	assert.Equal(t, 2*time.Second, uc.RetryBaseDelay)
	assert.Equal(t, 30*time.Second, uc.RetryMaxDelay)
}

func TestHealth_newUseCase(t *testing.T) {
	t.Parallel()

	lifecycle := mock.NewLifecycleState(t)
	checker := mock.NewDependencyChecker(t)

	h := Health{Enabled: true}

	uc := h.newUseCase(lifecycle, checker)
	require.NotNil(t, uc)
}

func TestConfig_NewMigrationsUseCase(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	migrations := mock.NewMigrations(t)
	clock := mock.NewClock(t)
	logger := mock.NewLogger(t)

	cfg := &Config{
		Usecases: Usecases{
			Migrations: Migrations{FilesDir: "/data/migrations"},
		},
	}

	uc, err := cfg.NewMigrationsUseCase(pool, migrations, clock, logger)
	require.NoError(t, err)
	require.NotNil(t, uc)
}

func TestConfig_NewOutboxPublisherUseCase(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	outbox := mock.NewOutbox(t)
	publisher := mock.NewPublisher(t)
	clock := mock.NewClock(t)
	logger := mock.NewLogger(t)

	cfg := &Config{
		Usecases: Usecases{
			OutboxPublisher: OutboxPublisher{
				WorkerID:       "bootstrap-worker",
				BatchSize:      10,
				RetryBaseDelay: time.Second,
				RetryMaxDelay:  time.Minute,
			},
		},
	}

	uc := cfg.NewOutboxPublisherUseCase(
		pool,
		outbox,
		publisher,
		clock,
		logger,
	)
	require.NotNil(t, uc)
	assert.Equal(t, "bootstrap-worker", uc.WorkerID)
}

func TestConfig_NewHealthUseCase(t *testing.T) {
	t.Parallel()

	lifecycle := mock.NewLifecycleState(t)
	checker := mock.NewDependencyChecker(t)

	cfg := &Config{
		Usecases: Usecases{
			Health: Health{Enabled: true},
		},
	}

	uc := cfg.NewHealthUseCase(lifecycle, checker)
	require.NotNil(t, uc)
}

func TestContract_newUseCase_Validation(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	clock := mock.NewClock(t)
	checkoutPayment := mock.NewCheckoutPaymentClient(t)
	checkoutPolicies, err := checkoutcatalog.NewRegistry()
	require.NoError(t, err)

	t.Run("invalid processing grace rejected", func(t *testing.T) {
		t.Parallel()

		_, err := (Contract{
			PaymentResultProcessingGracePeriod: 61 * time.Minute,
			ExpiryBatchSize:                    100,
		}).newUseCase(
			pool,
			mock.NewCustomerProfile(t),
			mock.NewMerchantProfile(t),
			mock.NewCreditAccount(t),
			mock.NewDecision(t),
			mock.NewContract(t),
			mock.NewCheckoutDecisionAllocation(t),
			checkoutPolicies,
			mock.NewInstallment(t),
			mock.NewInstallmentCharge(t),
			mock.NewPaymentAttempt(t),
			checkoutPayment,
			mock.NewContractCancellationClient(t),
			mock.NewOutbox(t),
			clock,
			"bnpl.audit.events.v1",
			mock.NewIdentifierGenerator(t),
			"https://bnpl.example/payments/return",
			nil,
			nil,
			nil,
			mustTestJurisdictions(t),
			lognoop.New(),
			metricsnoop.New(),
			ContractCancellationSaga{},
			contractuc.AllowAllNewCreditUsage{},
		)
		require.Error(t, err)
		require.ErrorContains(t, err, "must not exceed")
	})

	t.Run("valid settings accepted", func(t *testing.T) {
		t.Parallel()

		contractRepo := mock.NewContract(t)
		creditAccount := mock.NewCreditAccount(t)
		cancellationClient := mock.NewContractCancellationClient(t)
		definition, err := contractuc.NewCancellationSagaDefinition(
			pool,
			contractRepo,
			creditAccount,
			cancellationClient,
			clock,
			time.Second,
			time.Minute,
		)
		require.NoError(t, err)

		uc, err := (Contract{
			PaymentResultProcessingGracePeriod: 5 * time.Minute,
			ExpiryBatchSize:                    100,
		}).newUseCase(
			pool,
			mock.NewCustomerProfile(t),
			mock.NewMerchantProfile(t),
			creditAccount,
			mock.NewDecision(t),
			contractRepo,
			mock.NewCheckoutDecisionAllocation(t),
			checkoutPolicies,
			mock.NewInstallment(t),
			mock.NewInstallmentCharge(t),
			mock.NewPaymentAttempt(t),
			checkoutPayment,
			cancellationClient,
			mock.NewOutbox(t),
			clock,
			"bnpl.audit.events.v1",
			mock.NewIdentifierGenerator(t),
			"https://bnpl.example/payments/return",
			mock.NewSaga(t),
			mock.NewExecutor(t),
			definition,
			mustTestJurisdictions(t),
			lognoop.New(),
			metricsnoop.New(),
			ContractCancellationSaga{},
			contractuc.AllowAllNewCreditUsage{},
		)
		require.NoError(t, err)
		require.NotNil(t, uc)
	})
}

func TestCustomerCredit_newUseCase(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	customerProfile := mock.NewCustomerProfile(t)
	creditAccount := mock.NewCreditAccount(t)
	decision := mock.NewDecision(t)
	clock := mock.NewClock(t)
	client := mock.NewCreditAssessmentClient(t)

	initialLimitPolicies, err := initialcreditcatalog.NewRegistry()
	require.NoError(t, err)

	uc, err := (CustomerCredit{}).newUseCase(
		pool,
		customerProfile,
		creditAccount,
		decision,
		mock.NewCustomerRestriction(t),
		mock.NewInstallment(t),
		mock.NewDefaultEpisode(t),
		mock.NewOutbox(t),
		clock,
		sha256fingerprint.New(),
		client,
		mock.NewPolicyAssignment(t),
		initialLimitPolicies,
		initialcreditcatalog.DefaultRef(),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
	)
	require.NoError(t, err)
	require.NotNil(t, uc)
}

func TestFinancialCorePaymentReconciliation_effectiveRetryDelays(t *testing.T) {
	t.Parallel()

	fallback := PaymentAttemptReconciliation{
		RetryBaseDelay: 3 * time.Second,
		RetryMaxDelay:  90 * time.Second,
	}

	t.Run("uses own values when configured", func(t *testing.T) {
		t.Parallel()

		f := FinancialCorePaymentReconciliation{
			RetryBaseDelay: time.Second,
			RetryMaxDelay:  time.Minute,
		}

		base, maxDelay := f.effectiveRetryDelays(fallback)
		assert.Equal(t, time.Second, base)
		assert.Equal(t, time.Minute, maxDelay)
	})

	t.Run("falls back to payment attempt reconciliation values when unset", func(t *testing.T) {
		t.Parallel()

		f := FinancialCorePaymentReconciliation{}

		base, maxDelay := f.effectiveRetryDelays(fallback)
		assert.Equal(t, fallback.RetryBaseDelay, base)
		assert.Equal(t, fallback.RetryMaxDelay, maxDelay)
	})

	t.Run("falls back per-field when only one value is unset", func(t *testing.T) {
		t.Parallel()

		f := FinancialCorePaymentReconciliation{
			RetryBaseDelay: 5 * time.Second,
		}

		base, maxDelay := f.effectiveRetryDelays(fallback)
		assert.Equal(t, 5*time.Second, base)
		assert.Equal(t, fallback.RetryMaxDelay, maxDelay)
	})
}

func TestDecision_newUseCase_WiresCheckoutPolicy(t *testing.T) {
	t.Parallel()

	checkoutPolicies, err := checkoutcatalog.NewRegistry()
	require.NoError(t, err)

	idGen := mock.NewIdentifierGenerator(t)

	uc, err := (Decision{}).newUseCase(
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
		checkoutPolicies,
		checkoutcatalog.DefaultRef(),
		"bnpl.audit.events.v1",
		idGen,
		mustTestJurisdictions(t),
		metricsnoop.New(),
		decisionuc.AllowAllNewCreditUsage{},
	)
	require.NoError(t, err)
	require.NotNil(t, uc)
}

func mustTestJurisdictions(t *testing.T) *domaincapability.Jurisdictions {
	t.Helper()

	caps, err := domaincapability.NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction})
	require.NoError(t, err)

	return caps
}
