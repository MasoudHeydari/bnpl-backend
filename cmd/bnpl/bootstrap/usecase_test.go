package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewUseCases_NoopConfig(t *testing.T) {
	t.Parallel()

	app := testAppWithInfrastructure(t, noopTestConfig())

	usecases, err := app.NewUseCases()
	require.NoError(t, err)

	require.NotNil(t, usecases.OutboxPublisher)
	require.NotNil(t, usecases.Health)
	require.NotNil(t, usecases.Decision)
	require.NotNil(t, usecases.InstallmentLifecycle)
	require.NotNil(t, usecases.InstallmentRepayment)
	require.NotNil(t, usecases.FinancialCorePaymentResult)
	require.NotNil(t, usecases.FinancialCorePaymentSession)
	require.NotNil(t, usecases.FinancialCorePaymentReconciliation)
	require.NotNil(t, usecases.PaymentAttemptReconciliation)
	require.NotNil(t, usecases.SagaExecutor)
	require.NotNil(t, usecases.Contract)
	require.NotNil(t, usecases.Merchant)
	require.NotNil(t, usecases.MerchantTerminal)
	require.NotNil(t, usecases.CustomerAccount)
	require.NotNil(t, usecases.CustomerCredit)
}

func TestNewUseCases_InjectsExternalWebhookPublisher(t *testing.T) {
	t.Parallel()

	app := testAppWithInfrastructure(t, noopTestConfig())
	require.NotNil(t, app.External.WebhookPublisher)

	usecases, err := app.NewUseCases()
	require.NoError(t, err)
	require.NotNil(t, usecases.OutboxPublisher)
}
