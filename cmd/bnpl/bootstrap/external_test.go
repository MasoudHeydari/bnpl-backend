package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewExternal_NoopConfig(t *testing.T) {
	t.Parallel()

	app := testAppWithInfrastructure(t, noopTestConfig())

	require.NotNil(t, app.External.CreditAssessmentClient)
	require.NotNil(t, app.External.CheckoutPaymentClient)
	require.NotNil(t, app.External.InstallmentPaymentClient)
}
