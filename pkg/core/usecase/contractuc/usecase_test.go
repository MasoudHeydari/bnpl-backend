package contractuc

import (
	"testing"
	"time"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	extcontractcancellation "github.com/parstis/bnpl-backend/pkg/core/external/contractcancellation"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

func testCancellationSagaSettings() CancellationSagaSettings {
	return CancellationSagaSettings{
		RetryBaseDelay: time.Second,
		RetryMaxDelay:  time.Minute,
	}
}

func mustTestCancellationSagaDeps(
	t *testing.T,
	pool repo.Pool,
	contractRepo repo.Contract,
	creditAccount repo.CreditAccount,
	client extcontractcancellation.Client,
	clk clock.Clock,
) (repo.Saga, saga.Executor, saga.Definition) {
	t.Helper()

	settings := testCancellationSagaSettings()
	definition, err := NewCancellationSagaDefinition(
		pool,
		contractRepo,
		creditAccount,
		client,
		clk,
		settings.RetryBaseDelay,
		settings.RetryMaxDelay,
	)
	require.NoError(t, err)

	return mock.NewSaga(t), mock.NewExecutor(t), definition
}

func TestNew_ConfigValidation(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	clock := mock.NewClock(t)
	client := mock.NewCheckoutPaymentClient(t)
	contractRepo := mock.NewContract(t)
	cancellationClient := mock.NewContractCancellationClient(t)

	newUC := func(t *testing.T, grace time.Duration, batchSize int) (*UseCase, error) {
		t.Helper()
		sagaRepo, sagaExecutor, sagaDefinition := mustTestCancellationSagaDeps(
			t,
			pool,
			contractRepo,
			mock.NewCreditAccount(t),
			cancellationClient,
			clock,
		)
		return New(
			pool,
			mock.NewCustomerProfile(t),
			mock.NewMerchantProfile(t),
			mock.NewCreditAccount(t),
			mock.NewDecision(t),
			contractRepo,
			mock.NewCheckoutDecisionAllocation(t),
			mustTestCheckoutRegistry(t),
			mock.NewInstallment(t),
			mock.NewInstallmentCharge(t),
			mock.NewPaymentAttempt(t),
			client,
			cancellationClient,
			mock.NewOutbox(t),
			clock,
			"audit-events",
			mock.NewIdentifierGenerator(t),
			"https://bnpl.example/payments/return",
			grace,
			batchSize,
			testCancellationSagaSettings(),
			sagaRepo,
			sagaExecutor,
			sagaDefinition,
			mustTestOriginationJurisdictions(t),
			lognoop.New(),
			metricsnoop.New(),
			AllowAllNewCreditUsage{},
		)
	}

	t.Run("zero processing grace accepted", func(t *testing.T) {
		t.Parallel()

		uc, err := newUC(t, 0, 1)
		require.NoError(t, err)
		require.NotNil(t, uc)
	})

	t.Run("five minutes accepted", func(t *testing.T) {
		t.Parallel()

		uc, err := newUC(t, 5*time.Minute, 100)
		require.NoError(t, err)
		require.Equal(t, 5*time.Minute, uc.PaymentResultProcessingGracePeriod)
		require.Equal(t, 100, uc.ExpiryBatchSize)
	})

	t.Run("sixty minutes accepted", func(t *testing.T) {
		t.Parallel()

		_, err := newUC(t, model.MaxPaymentResultProcessingGracePeriod, 100)
		require.NoError(t, err)
	})

	t.Run("negative processing grace rejected", func(t *testing.T) {
		t.Parallel()

		_, err := newUC(t, -time.Second, 100)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not be negative")
	})

	t.Run("greater than sixty minutes rejected", func(t *testing.T) {
		t.Parallel()

		_, err := newUC(t, model.MaxPaymentResultProcessingGracePeriod+time.Second, 100)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not exceed")
	})

	t.Run("zero batch size rejected", func(t *testing.T) {
		t.Parallel()

		_, err := newUC(t, time.Minute, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "batch size must be positive")
	})

	t.Run("negative batch size rejected", func(t *testing.T) {
		t.Parallel()

		_, err := newUC(t, time.Minute, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "batch size must be positive")
	})

	t.Run("wires cancellation saga settings", func(t *testing.T) {
		t.Parallel()

		settings := CancellationSagaSettings{
			RetryBaseDelay: 2 * time.Second,
			RetryMaxDelay:  45 * time.Second,
		}
		definition, err := NewCancellationSagaDefinition(
			pool,
			contractRepo,
			mock.NewCreditAccount(t),
			cancellationClient,
			clock,
			settings.RetryBaseDelay,
			settings.RetryMaxDelay,
		)
		require.NoError(t, err)
		uc, err := New(
			pool,
			mock.NewCustomerProfile(t),
			mock.NewMerchantProfile(t),
			mock.NewCreditAccount(t),
			mock.NewDecision(t),
			contractRepo,
			mock.NewCheckoutDecisionAllocation(t),
			mustTestCheckoutRegistry(t),
			mock.NewInstallment(t),
			mock.NewInstallmentCharge(t),
			mock.NewPaymentAttempt(t),
			client,
			cancellationClient,
			mock.NewOutbox(t),
			clock,
			"audit-events",
			mock.NewIdentifierGenerator(t),
			"https://bnpl.example/payments/return",
			5*time.Minute,
			100,
			settings,
			mock.NewSaga(t),
			mock.NewExecutor(t),
			definition,
			mustTestOriginationJurisdictions(t),
			lognoop.New(),
			metricsnoop.New(),
			AllowAllNewCreditUsage{},
		)
		require.NoError(t, err)
		require.Equal(t, settings, uc.CancellationSaga)
	})

	t.Run("rejects retry max less than retry base", func(t *testing.T) {
		t.Parallel()

		settings := testCancellationSagaSettings()
		settings.RetryBaseDelay = time.Minute
		settings.RetryMaxDelay = time.Second
		_, err := New(
			pool,
			mock.NewCustomerProfile(t),
			mock.NewMerchantProfile(t),
			mock.NewCreditAccount(t),
			mock.NewDecision(t),
			mock.NewContract(t),
			mock.NewCheckoutDecisionAllocation(t),
			mustTestCheckoutRegistry(t),
			mock.NewInstallment(t),
			mock.NewInstallmentCharge(t),
			mock.NewPaymentAttempt(t),
			client,
			mock.NewContractCancellationClient(t),
			mock.NewOutbox(t),
			clock,
			"audit-events",
			mock.NewIdentifierGenerator(t),
			"https://bnpl.example/payments/return",
			5*time.Minute,
			100,
			settings,
			nil,
			nil,
			nil,
			mustTestOriginationJurisdictions(t),
			lognoop.New(),
			metricsnoop.New(),
			AllowAllNewCreditUsage{},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "retry max delay")
	})

	t.Run("requires origination jurisdictions", func(t *testing.T) {
		t.Parallel()

		sagaRepo, sagaExecutor, sagaDefinition := mustTestCancellationSagaDeps(
			t,
			pool,
			contractRepo,
			mock.NewCreditAccount(t),
			cancellationClient,
			clock,
		)
		_, err := New(
			pool,
			mock.NewCustomerProfile(t),
			mock.NewMerchantProfile(t),
			mock.NewCreditAccount(t),
			mock.NewDecision(t),
			contractRepo,
			mock.NewCheckoutDecisionAllocation(t),
			mustTestCheckoutRegistry(t),
			mock.NewInstallment(t),
			mock.NewInstallmentCharge(t),
			mock.NewPaymentAttempt(t),
			client,
			cancellationClient,
			mock.NewOutbox(t),
			clock,
			"audit-events",
			mock.NewIdentifierGenerator(t),
			"https://bnpl.example/payments/return",
			5*time.Minute,
			100,
			testCancellationSagaSettings(),
			sagaRepo,
			sagaExecutor,
			sagaDefinition,
			nil,
			lognoop.New(),
			metricsnoop.New(),
			AllowAllNewCreditUsage{},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "origination jurisdictions is required")
	})

	t.Run("requires cancellation saga dependencies", func(t *testing.T) {
		t.Parallel()

		_, err := New(
			pool,
			mock.NewCustomerProfile(t),
			mock.NewMerchantProfile(t),
			mock.NewCreditAccount(t),
			mock.NewDecision(t),
			contractRepo,
			mock.NewCheckoutDecisionAllocation(t),
			mustTestCheckoutRegistry(t),
			mock.NewInstallment(t),
			mock.NewInstallmentCharge(t),
			mock.NewPaymentAttempt(t),
			client,
			cancellationClient,
			mock.NewOutbox(t),
			clock,
			"audit-events",
			mock.NewIdentifierGenerator(t),
			"https://bnpl.example/payments/return",
			5*time.Minute,
			100,
			testCancellationSagaSettings(),
			nil,
			nil,
			nil,
			mustTestOriginationJurisdictions(t),
			lognoop.New(),
			metricsnoop.New(),
			AllowAllNewCreditUsage{},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cancellation saga repository is required")
	})
}
