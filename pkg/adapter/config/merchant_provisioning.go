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

// MerchantProvisioning configures the external participation integration.
// Only is a temporary execution restriction, independent of the deployment environment.
type MerchantProvisioning struct {
	Enabled     bool   `yaml:"enabled"`
	Only        bool   `yaml:"only"`
	Environment string `yaml:"environment"`
	BaseURL     string `yaml:"base_url"`

	// Token is the service bearer credential, decoded through the shared configuration loader.
	Token model.Secret `yaml:"token" json:"-"`
}

// configureMerchantParticipationSubscription selects the existing provisioning-only subscription.
// It uses the decoded configuration and never reads the process environment.
func (c *Config) configureMerchantParticipationSubscription() {
	if c.MerchantProvisioning.Enabled {
		c.Infrastructure.Messaging.Kafka.Consumer.Topics = []string{MerchantParticipationTopic}
	}
}

// ValidateMerchantProvisioning rejects unsafe pilot configuration before wiring.
func (c *Config) ValidateMerchantProvisioning() error {
	const op = "config.Config.ValidateMerchantProvisioning"

	p := c.MerchantProvisioning
	if p.Enabled != p.Only {
		return fmt.Errorf("%s: the pilot requires provisioning enabled and provisioning-only mode together", op)
	}

	if !p.Enabled {
		return nil
	}

	env, err := c.Runtime.ParsedEnvironment()
	if err != nil {
		return err
	}

	if !buildprofile.IsDevelopment() || !env.IsDevelopment() {
		return fmt.Errorf("%s: Merchant provisioning pilot requires a development build and development runtime environment", op)
	}

	if p.Environment != "SANDBOX" {
		return fmt.Errorf("%s: only SANDBOX participation is admitted during the pilot", op)
	}

	if c.Infrastructure.CircuitBreaker.Provider != circuitbreaker.GoBreakerProvider {
		return fmt.Errorf("%s: Merchant provisioning pilot requires the gobreaker circuit breaker provider", op)
	}

	if c.External.FinancialCore.Provider != "noop" ||
		c.Infrastructure.Messaging.Provider != "kafka" ||
		c.Infrastructure.Task.Provider != "inprocess" ||
		c.Infrastructure.HTTPClient.Provider != "standard" {
		return fmt.Errorf("%s: Merchant provisioning pilot requires noop Financial Core, Kafka, inprocess tasks and standard HTTP", op)
	}

	k := c.Infrastructure.Messaging.Kafka
	if strings.TrimSpace(k.ClientID) == "" ||
		strings.TrimSpace(k.GroupID) == "" ||
		strings.TrimSpace(k.Consumer.QuarantineTopic) == "" ||
		k.Consumer.InitialOffset != "oldest" {
		return fmt.Errorf("%s: Merchant provisioning pilot requires a configured Kafka client, group, quarantine and oldest offset on a fresh topic", op)
	}

	if len(k.Consumer.Topics) != 1 || k.Consumer.Topics[0] != MerchantParticipationTopic {
		return fmt.Errorf("%s: Merchant provisioning pilot must consume only its participation topic", op)
	}

	if strings.TrimSpace(p.BaseURL) == "" || strings.TrimSpace(p.Token.Reveal()) == "" {
		return fmt.Errorf("%s: HTTPS origin and EXTERNAL_MERCHANT_HTTP_TOKEN are required", op)
	}

	return nil
}
