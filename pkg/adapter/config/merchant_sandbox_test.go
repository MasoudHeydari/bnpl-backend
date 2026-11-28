package config

import (
	"fmt"
	"testing"

	"github.com/parstis/bnpl-backend/internal/buildprofile"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/stretchr/testify/require"
)

func TestMerchantSandboxRequiresDevelopmentBuildAndRuntime(t *testing.T) {
	cfg := validConfig()
	cfg.MerchantSandbox = MerchantSandbox{
		Enabled: true,
		BaseURL: "https://merchant.internal",
		Token:   model.NewSecret("synthetic-token"),
	}
	cfg.Infrastructure.CircuitBreaker.Provider = circuitbreaker.GoBreakerProvider
	cfg.Infrastructure.HTTPClient.Provider = "standard"
	cfg.Infrastructure.Task.Provider = "inprocess"
	cfg.External.FinancialCore.Provider = "noop"
	cfg.Infrastructure.Messaging.Provider = "kafka"
	cfg.Infrastructure.Messaging.Kafka = Kafka{
		ClientID: "bnpl-sandbox",
		GroupID:  "bnpl-sandbox-consumer-group",
		Consumer: Consumer{
			Topics:          []string{MerchantParticipationTopic},
			InitialOffset:   "oldest",
			QuarantineTopic: "bnpl.sandbox.consumer.quarantine.v1",
		},
	}
	for _, env := range []string{"development", "production", "staging", "test"} {
		t.Run(env, func(t *testing.T) {
			cfg.Runtime.Environment = env
			err := cfg.ValidateMerchantSandbox()
			if env == "development" && buildprofile.IsDevelopment() {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, "requires a development build and development runtime environment")
		})
	}

	cfg.Runtime.Environment = "development"
	if buildprofile.IsDevelopment() {
		cfg.Infrastructure.CircuitBreaker.Provider = circuitbreaker.NoopProvider
		require.ErrorContains(t, cfg.ValidateMerchantSandbox(), "gobreaker")
		cfg.Infrastructure.CircuitBreaker.Provider = circuitbreaker.GoBreakerProvider
		cfg.MerchantSandbox.Token = model.NewSecret("")
		require.ErrorContains(t, cfg.ValidateMerchantSandbox(), "MERCHANT_SANDBOX_TOKEN")
	}

	cfg.MerchantSandbox.Enabled = false
	cfg.Runtime.Environment = "production"
	require.NoError(t, cfg.ValidateMerchantSandbox())
}

func TestMerchantSandboxTokenIsNotPrinted(t *testing.T) {
	const token = "synthetic-service-token"
	cfg := &Config{MerchantSandbox: MerchantSandbox{Token: model.NewSecret(token)}}
	require.NotContains(t, cfg.String(), token)
	require.NotContains(t, fmt.Sprintf("%+v", cfg.MerchantSandbox), token)
	require.NotContains(t, fmt.Sprintf("%#v", cfg.MerchantSandbox), token)
}
