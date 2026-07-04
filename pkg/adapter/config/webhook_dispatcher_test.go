package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/external/merchantwebhook"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	webhookpolicy "github.com/parstis/bnpl-backend/pkg/core/policy/webhook"
)

func TestWebhookPolicyForEnvironment(t *testing.T) {
	t.Parallel()

	dev := webhookPolicyForEnvironment(model.DevelopmentRuntimeEnvironment)
	require.Equal(t, webhookpolicy.Development(), dev)

	testEnv := webhookPolicyForEnvironment(model.TestRuntimeEnvironment)
	require.Equal(t, webhookpolicy.Development(), testEnv)

	prod := webhookPolicyForEnvironment(model.ProductionRuntimeEnvironment)
	require.Equal(t, webhookpolicy.Strict(), prod)

	unknown := webhookPolicyForEnvironment(model.RuntimeEnvironment("whatever"))
	require.Equal(t, webhookpolicy.Strict(), unknown)
}

func TestExternalWebhookDispatcher_ApplyDefaults(t *testing.T) {
	t.Parallel()

	var cfg ExternalWebhookDispatcher
	cfg.ApplyDefaults()
	require.Equal(t, string(merchantwebhook.HTTPProvider), cfg.Provider)
	require.Equal(t, 10*time.Second, cfg.HTTP.Timeout)
	require.Equal(t, 5*time.Second, cfg.HTTP.DialTimeout)
	require.Equal(t, int64(65536), cfg.HTTP.MaxResponseBytes)
}

func TestExternalWebhookDispatcher_Validate_HTTP(t *testing.T) {
	t.Parallel()

	cfg := ExternalWebhookDispatcher{
		Provider: string(merchantwebhook.HTTPProvider),
		HTTP: ExternalWebhookDispatcherHTTP{
			Timeout:          10 * time.Second,
			DialTimeout:      5 * time.Second,
			MaxResponseBytes: 65536,
		},
	}
	require.NoError(t, cfg.validate(model.DevelopmentRuntimeEnvironment))
}

func TestExternalWebhookDispatcher_Validate_HTTPDefaultsAllowed(t *testing.T) {
	t.Parallel()

	cfg := ExternalWebhookDispatcher{Provider: string(merchantwebhook.HTTPProvider)}
	require.NoError(t, cfg.validate(model.DevelopmentRuntimeEnvironment))
}

func TestExternalWebhookDispatcher_Validate_NoopDoesNotRequireHTTP(t *testing.T) {
	t.Parallel()

	cfg := ExternalWebhookDispatcher{Provider: string(merchantwebhook.NoopProvider)}
	require.NoError(t, cfg.validate(model.DevelopmentRuntimeEnvironment))
}

func TestExternalWebhookDispatcher_Validate_InvalidHTTPTimeout(t *testing.T) {
	t.Parallel()

	cfg := ExternalWebhookDispatcher{
		Provider: string(merchantwebhook.HTTPProvider),
		HTTP:     ExternalWebhookDispatcherHTTP{Timeout: -time.Second},
	}
	err := cfg.validate(model.DevelopmentRuntimeEnvironment)
	require.Error(t, err)
	require.Contains(t, err.Error(), "external.webhook_dispatcher.http.timeout")
}

func TestExternalWebhookDispatcher_Validate_InvalidDialTimeout(t *testing.T) {
	t.Parallel()

	cfg := ExternalWebhookDispatcher{
		Provider: string(merchantwebhook.HTTPProvider),
		HTTP:     ExternalWebhookDispatcherHTTP{DialTimeout: -time.Second},
	}
	err := cfg.validate(model.DevelopmentRuntimeEnvironment)
	require.Error(t, err)
	require.Contains(t, err.Error(), "external.webhook_dispatcher.http.dial_timeout")
}

func TestExternalWebhookDispatcher_Validate_InvalidMaxResponseBytes(t *testing.T) {
	t.Parallel()

	cfg := ExternalWebhookDispatcher{
		Provider: string(merchantwebhook.HTTPProvider),
		HTTP:     ExternalWebhookDispatcherHTTP{MaxResponseBytes: -1},
	}
	err := cfg.validate(model.DevelopmentRuntimeEnvironment)
	require.Error(t, err)
	require.Contains(t, err.Error(), "external.webhook_dispatcher.http.max_response_bytes")
}

func TestExternalWebhookDispatcher_Validate_UnsupportedProvider(t *testing.T) {
	t.Parallel()

	cfg := ExternalWebhookDispatcher{Provider: "rabbitmq"}
	err := cfg.validate(model.DevelopmentRuntimeEnvironment)
	require.Error(t, err)
	require.Contains(t, err.Error(), "external.webhook_dispatcher.provider")
}

func TestConfig_Validate_WebhookDispatcherHTTPSelection(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.External.WebhookDispatcher = ExternalWebhookDispatcher{
		Provider: string(merchantwebhook.HTTPProvider),
		HTTP: ExternalWebhookDispatcherHTTP{
			Timeout:          10 * time.Second,
			DialTimeout:      5 * time.Second,
			MaxResponseBytes: 65536,
		},
	}
	require.NoError(t, cfg.Validate())
}

func TestConfig_Validate_WebhookDispatcherNoopSelection(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.External.WebhookDispatcher = ExternalWebhookDispatcher{
		Provider: string(merchantwebhook.NoopProvider),
	}
	require.NoError(t, cfg.Validate())
}
