package config

import (
	"fmt"
	"testing"

	"github.com/parstis/bnpl-backend/internal/buildprofile"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/stretchr/testify/require"
)

func TestMerchantProvisioningRequiresDevelopmentBuildAndRuntime(t *testing.T) {
	cfg := validConfig()
	cfg.MerchantProvisioning = MerchantProvisioning{
		Enabled:     true,
		Only:        true,
		Environment: "SANDBOX",
		BaseURL:     "https://merchant.internal",
		Token:       model.NewSecret("synthetic-token"),
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
			err := cfg.ValidateMerchantProvisioning()
			if env == "development" && buildprofile.IsDevelopment() {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, "requires a development build and development runtime environment")
		})
	}

	cfg.Runtime.Environment = "development"
	if buildprofile.IsDevelopment() {
		cfg.MerchantProvisioning.Only = false
		require.ErrorContains(t, cfg.ValidateMerchantProvisioning(), "provisioning-only mode")
		cfg.MerchantProvisioning.Only = true
		cfg.MerchantProvisioning.Environment = "LIVE"
		require.ErrorContains(t, cfg.ValidateMerchantProvisioning(), "only SANDBOX")
		cfg.MerchantProvisioning.Environment = "SANDBOX"
		cfg.Infrastructure.CircuitBreaker.Provider = circuitbreaker.NoopProvider
		require.ErrorContains(t, cfg.ValidateMerchantProvisioning(), "gobreaker")
		cfg.Infrastructure.CircuitBreaker.Provider = circuitbreaker.GoBreakerProvider
		cfg.MerchantProvisioning.Token = model.NewSecret("")
		require.ErrorContains(t, cfg.ValidateMerchantProvisioning(), "EXTERNAL_MERCHANT_HTTP_TOKEN")
	}

	cfg.MerchantProvisioning.Enabled = false
	cfg.MerchantProvisioning.Only = false
	cfg.Runtime.Environment = "production"
	require.NoError(t, cfg.ValidateMerchantProvisioning())
}

func TestMerchantProvisioningTokenIsNotPrinted(t *testing.T) {
	const token = "synthetic-service-token"
	cfg := &Config{MerchantProvisioning: MerchantProvisioning{Token: model.NewSecret(token)}}
	require.NotContains(t, cfg.String(), token)
	require.NotContains(t, fmt.Sprintf("%+v", cfg.MerchantProvisioning), token)
	require.NotContains(t, fmt.Sprintf("%#v", cfg.MerchantProvisioning), token)
}

func TestLoadMerchantProvisioningToken(t *testing.T) {
	skipIfProductionBinary(t)

	const token = "synthetic-service-token"
	t.Setenv("EXTERNAL_MERCHANT_HTTP_TOKEN", token)

	cfg, err := Load(writeConfigFile(t, validVersionsYAML+validMinimalValidationYAML+`
merchant_provisioning:
  enabled: false
  only: false
  token: ${EXTERNAL_MERCHANT_HTTP_TOKEN}
`))
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, token, cfg.MerchantProvisioning.Token.Reveal())
	require.NotContains(t, cfg.String(), token)
}

func TestMerchantParticipationSubscription(t *testing.T) {
	cfg := validConfig()
	topics := []string{"payment-results", "reconciliation-results"}
	cfg.Infrastructure.Messaging.Kafka.Consumer.Topics = topics

	cfg.configureMerchantParticipationSubscription()
	require.Equal(t, topics, cfg.Infrastructure.Messaging.Kafka.Consumer.Topics)

	cfg.MerchantProvisioning.Enabled = true
	cfg.configureMerchantParticipationSubscription()
	require.Equal(t, []string{MerchantParticipationTopic}, cfg.Infrastructure.Messaging.Kafka.Consumer.Topics)
}
