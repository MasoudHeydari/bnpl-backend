package config

import (
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/internal/buildprofile"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// MerchantParticipationTopic is the exact topic in the agreed v1 contract.
const MerchantParticipationTopic = "merchant.business.line.participation.requests.v1"

// MerchantSandbox is an opt-in, provisioning-only runtime; it is not LIVE.
type MerchantSandbox struct {
	Enabled bool   `yaml:"enabled"`
	BaseURL string `yaml:"base_url"`
	// Token is read directly from the environment, never from YAML or a file.
	Token model.Secret `yaml:"-" json:"-"`
}

// ValidateMerchantSandbox rejects unsafe pilot configuration before wiring.
func (c *Config) ValidateMerchantSandbox() error {
	const op = "config.Config.ValidateMerchantSandbox"
	p := c.MerchantSandbox
	if !p.Enabled {
		return nil
	}

	env, err := c.Runtime.ParsedEnvironment()
	if err != nil {
		return err
	}

	if !buildprofile.IsDevelopment() || !env.IsDevelopment() {
		return fmt.Errorf("%s: Sandbox requires a development build and development runtime environment", op)
	}

	if c.Infrastructure.CircuitBreaker.Provider != circuitbreaker.GoBreakerProvider {
		return fmt.Errorf("%s: Sandbox requires the gobreaker circuit breaker provider", op)
	}

	if c.External.FinancialCore.Provider != "noop" || c.Infrastructure.Messaging.Provider != "kafka" || c.Infrastructure.Task.Provider != "inprocess" || c.Infrastructure.HTTPClient.Provider != "standard" {
		return fmt.Errorf("%s: Sandbox requires noop Financial Core, Kafka, inprocess tasks and standard HTTP", op)
	}

	k := c.Infrastructure.Messaging.Kafka
	if k.ClientID != "bnpl-sandbox" || k.GroupID != "bnpl-sandbox-consumer-group" || k.Consumer.QuarantineTopic != "bnpl.sandbox.consumer.quarantine.v1" || k.Consumer.InitialOffset != "oldest" {
		return fmt.Errorf("%s: Sandbox requires its agreed Kafka identity, quarantine and oldest offset on a fresh topic", op)
	}

	if len(k.Consumer.Topics) != 1 || k.Consumer.Topics[0] != MerchantParticipationTopic {
		return fmt.Errorf("%s: Sandbox must consume only its participation topic", op)
	}

	if strings.TrimSpace(p.BaseURL) == "" || strings.TrimSpace(p.Token.Reveal()) == "" {
		return fmt.Errorf("%s: HTTPS origin and MERCHANT_SANDBOX_TOKEN are required", op)
	}

	return nil
}
