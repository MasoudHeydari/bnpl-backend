package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/external/merchantwebhook"
	merchantwebhookhttp "github.com/parstis/bnpl-backend/pkg/adapter/external/merchantwebhook/http"
	messagingrouter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/router"
)

func TestNewExternal_NoopConfig(t *testing.T) {
	t.Parallel()

	app := testAppWithInfrastructure(t, noopTestConfig())

	require.NotNil(t, app.External.CreditAssessmentClient)
	require.NotNil(t, app.External.CheckoutPaymentClient)
	require.NotNil(t, app.External.InstallmentPaymentClient)
	require.NotNil(t, app.External.WebhookPublisher)
}

func TestNewExternal_WebhookPublisherUsesNoopProvider(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.External.WebhookDispatcher.Provider = string(merchantwebhook.NoopProvider)

	app := testAppWithInfrastructure(t, cfg)
	require.NotNil(t, app.External.WebhookPublisher)

	_, isRouter := app.External.WebhookPublisher.(*messagingrouter.Router)
	require.True(t, isRouter)

	require.Empty(t, app.External.closers)
}

func TestNewExternal_WebhookPublisherUsesHTTPProviderWithoutOwningHTTPClient(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.External.WebhookDispatcher = config.ExternalWebhookDispatcher{
		Provider: string(merchantwebhook.HTTPProvider),
		HTTP: config.ExternalWebhookDispatcherHTTP{
			Timeout:          merchantwebhookhttp.DefaultTimeout,
			DialTimeout:      merchantwebhookhttp.DefaultDialTimeout,
			MaxResponseBytes: merchantwebhookhttp.DefaultMaxResponseBytes,
		},
	}

	app := testAppWithInfrastructure(t, cfg)
	require.NotNil(t, app.External.WebhookPublisher)

	_, isRouter := app.External.WebhookPublisher.(*messagingrouter.Router)
	require.True(t, isRouter)

	// Shared infrastructure HTTP client owns transports; External must not
	// close it via a webhook deliverer closer.
	require.Empty(t, app.External.closers)
	require.NoError(t, app.External.Close())
}
