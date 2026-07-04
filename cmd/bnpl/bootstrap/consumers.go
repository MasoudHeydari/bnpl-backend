package bootstrap

import (
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	adaptermessaging "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers/paymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers/paymentresult"
)

// RegisterConsumers registers startup-known Kafka message handlers.
//
// Topic → handler bindings are fixed at bootstrap from configuration. Dynamic
// or per-request consumer registration does not belong here.
//
// Financial Core confirmed there is no installment payment-session-created
// Kafka topic; session recovery is sync CreateCheckoutPayment with the original
// idempotency key (plus abandoned-created job failure). Only payment-result and
// reconciliation-result consumers are registered for payment flows.
func (a *App) RegisterConsumers() (*consumers.Registry, error) {
	const op = "bootstrap.App.RegisterConsumers"

	if err := validateConsumerSubscriptions(a.Config); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	registry, err := consumers.NewRegistry(a.Infrastructure.Logger, a.Infrastructure.ErrorReporter)
	if err != nil {
		return nil, fmt.Errorf("%s: create consumer registry: %w", op, err)
	}

	paymentResultHandler := paymentresult.New(
		a.UseCases.FinancialCorePaymentResult,
		a.Infrastructure.Logger,
	)

	if err := registry.Register(
		a.Config.PaymentResultsTopic(),
		paymentResultHandler.Handle,
	); err != nil {
		return nil, fmt.Errorf("%s: register payment result consumer: %w", op, err)
	}

	paymentReconciliationHandler := paymentreconciliation.New(
		a.UseCases.FinancialCorePaymentReconciliation,
		a.Infrastructure.Logger,
	)

	if err := registry.Register(
		a.Config.InstallmentPaymentReconciliationResultsTopic(),
		paymentReconciliationHandler.Handle,
	); err != nil {
		return nil, fmt.Errorf("%s: register payment reconciliation consumer: %w", op, err)
	}

	return registry, nil
}

// validateConsumerSubscriptions makes unhandled Kafka topics a startup error
// rather than silently consuming and quarantining payment messages.
func validateConsumerSubscriptions(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("consumer subscription validation: config is required")
	}

	m := cfg.Infrastructure.Messaging
	if m.Provider != adaptermessaging.KafkaProvider {
		return nil
	}

	expected := map[string]bool{
		strings.TrimSpace(cfg.PaymentResultsTopic()):                          false,
		strings.TrimSpace(cfg.InstallmentPaymentReconciliationResultsTopic()): false,
	}
	if len(expected) != 2 {
		return fmt.Errorf("consumer subscription validation: payment result topics must be distinct")
	}
	for topic := range expected {
		if topic == "" {
			return fmt.Errorf("consumer subscription validation: registered topic must be nonempty")
		}
	}

	for _, raw := range m.Kafka.Consumer.Topics {
		topic := strings.TrimSpace(raw)
		seen, registered := expected[topic]
		if !registered {
			return fmt.Errorf("consumer subscription validation: topic %q has no registered handler", topic)
		}
		if seen {
			return fmt.Errorf("consumer subscription validation: duplicate consumer topic %q", topic)
		}
		expected[topic] = true
	}
	for topic, seen := range expected {
		if !seen {
			return fmt.Errorf("consumer subscription validation: missing subscribed topic %q", topic)
		}
	}
	return nil
}
