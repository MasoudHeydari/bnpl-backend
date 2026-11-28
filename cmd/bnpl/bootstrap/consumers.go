package bootstrap

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	adaptermessaging "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers/merchantparticipation"
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

	if a.Config.MerchantSandbox.Enabled {
		handler, err := merchantparticipation.New(a.MerchantSandbox, a.Infrastructure.FingerprintHasher)
		if err != nil {
			return nil, fmt.Errorf("%s: create merchant participation handler: %w", op, err)
		}

		err = registry.Register(config.MerchantParticipationTopic, handler.Handle)
		if err != nil {
			return nil, fmt.Errorf("%s: register merchant participation handler: %w", op, err)
		}

		return registry, nil
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

// consumerTopicBinding pairs a handler registration topic (KAFKA_TOPIC_*) with
// the matching Kafka consumer subscription env (KAFKA_CONSUMER_TOPIC_*).
type consumerTopicBinding struct {
	name         string
	handlerEnv   string
	consumerEnv  string
	handlerTopic string
}

// validateConsumerSubscriptions makes unhandled or misconfigured Kafka topics a
// startup error rather than silently consuming and quarantining payment messages.
//
// Empty values and handler/consumer mismatches are reported separately so ops can
// tell a missing env from a typo between KAFKA_TOPIC_* and KAFKA_CONSUMER_TOPIC_*.
func validateConsumerSubscriptions(cfg *config.Config) error {
	const op = "consumer subscription validation"

	if cfg == nil {
		return fmt.Errorf("%s: config is required", op)
	}

	if cfg.MerchantSandbox.Enabled {
		topics := cfg.Infrastructure.Messaging.Kafka.Consumer.Topics
		if len(topics) != 1 || topics[0] != config.MerchantParticipationTopic {
			return fmt.Errorf("%s: Sandbox must subscribe only to Merchant participation", op)
		}

		return nil
	}

	m := cfg.Infrastructure.Messaging
	if m.Provider != adaptermessaging.KafkaProvider {
		return nil
	}

	bindings := []consumerTopicBinding{
		{
			name:         "payment_results",
			handlerEnv:   "KAFKA_TOPIC_PAYMENT_RESULTS",
			consumerEnv:  "KAFKA_CONSUMER_TOPIC_PAYMENT_RESULTS",
			handlerTopic: strings.TrimSpace(cfg.PaymentResultsTopic()),
		},
		{
			name:         "installment_payment_reconciliation_results",
			handlerEnv:   "KAFKA_TOPIC_INSTALLMENT_PAYMENT_RECONCILIATION_RESULTS",
			consumerEnv:  "KAFKA_CONSUMER_TOPIC_INSTALLMENT_PAYMENT_RECONCILIATION_RESULTS",
			handlerTopic: strings.TrimSpace(cfg.InstallmentPaymentReconciliationResultsTopic()),
		},
	}

	for _, b := range bindings {
		if b.handlerTopic == "" {
			return fmt.Errorf(
				"%s: handler topic for %s is empty (%s must be set)",
				op,
				b.name,
				b.handlerEnv,
			)
		}
	}

	if bindings[0].handlerTopic == bindings[1].handlerTopic {
		return fmt.Errorf(
			"%s: handler topics must be distinct: %s and %s both resolve to %q",
			op,
			bindings[0].handlerEnv,
			bindings[1].handlerEnv,
			bindings[0].handlerTopic,
		)
	}

	subscribed := make([]string, 0, len(m.Kafka.Consumer.Topics))
	seenSubscribed := make(map[string]struct{}, len(m.Kafka.Consumer.Topics))

	for _, raw := range m.Kafka.Consumer.Topics {
		topic := strings.TrimSpace(raw)
		if topic == "" {
			return fmt.Errorf(
				"%s: kafka.consumer.topics contains an empty entry (check %s and %s)",
				op,
				bindings[0].consumerEnv,
				bindings[1].consumerEnv,
			)
		}

		if _, dup := seenSubscribed[topic]; dup {
			return fmt.Errorf("%s: duplicate subscribed topic %q", op, topic)
		}

		seenSubscribed[topic] = struct{}{}
		subscribed = append(subscribed, topic)
	}

	if len(subscribed) == 0 {
		return fmt.Errorf(
			"%s: kafka.consumer.topics is empty (set %s and %s)",
			op,
			bindings[0].consumerEnv,
			bindings[1].consumerEnv,
		)
	}

	handlerByTopic := make(map[string]consumerTopicBinding, len(bindings))
	for _, b := range bindings {
		handlerByTopic[b.handlerTopic] = b
	}

	for _, topic := range subscribed {
		if _, ok := handlerByTopic[topic]; ok {
			continue
		}

		return fmt.Errorf(
			"%s: subscribed topic %q has no matching handler topic "+
				"(configured handlers: %s=%q from %s, %s=%q from %s; "+
				"each %s must equal its matching %s — this is usually a typo or env mismatch)",
			op,
			topic,
			bindings[0].name,
			bindings[0].handlerTopic,
			bindings[0].handlerEnv,
			bindings[1].name,
			bindings[1].handlerTopic,
			bindings[1].handlerEnv,
			"KAFKA_CONSUMER_TOPIC_*",
			"KAFKA_TOPIC_*",
		)
	}

	for _, b := range bindings {
		if _, ok := seenSubscribed[b.handlerTopic]; ok {
			continue
		}

		return fmt.Errorf(
			"%s: handler topic for %s is %q (%s) but it is not subscribed in kafka.consumer.topics "+
				"(set %s to the same value; currently subscribed: %s)",
			op,
			b.name,
			b.handlerTopic,
			b.handlerEnv,
			b.consumerEnv,
			formatTopicList(subscribed),
		)
	}

	return nil
}

func formatTopicList(topics []string) string {
	if len(topics) == 0 {
		return "(none)"
	}

	quoted := make([]string, 0, len(topics))
	for _, topic := range topics {
		quoted = append(quoted, strconv.Quote(topic))
	}

	return strings.Join(quoted, ", ")
}
