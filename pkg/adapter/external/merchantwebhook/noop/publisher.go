package noop

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/metrics"
	"github.com/parstis/bnpl-backend/pkg/core/webhook"
)

// OutcomeSkipped is the business-metric label for an intentionally skipped
// webhook delivery. It is distinct from "delivered" so operators never confuse
// a noop acknowledgement with a successful merchant HTTP POST.
const OutcomeSkipped = "skipped"

// Publisher acknowledges webhook:// outbox rows without network I/O.
type Publisher struct {
	logger          log.Logger
	businessMetrics metrics.BusinessMetrics
}

var _ messaging.Publisher = (*Publisher)(nil)

// NewPublisher constructs a noop webhook publisher. Every dependency is required.
func NewPublisher(logger log.Logger, businessMetrics metrics.BusinessMetrics) (*Publisher, error) {
	const op = "merchantwebhooknoop.NewPublisher"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	if businessMetrics == nil {
		return nil, fmt.Errorf("%s: business metrics is required", op)
	}

	return &Publisher{logger: logger, businessMetrics: businessMetrics}, nil
}

// Publish records an explicit skip. The outbox publisher persists status
// "skipped" (not "published") so operators never confuse noop with a real
// merchant HTTP delivery. Kafka destinations are unaffected because this
// publisher is only routed for webhook:// prefixes.
func (p *Publisher) Publish(ctx context.Context, destination string, msg messaging.Message) error {
	const op = "merchantwebhooknoop.Publisher.Publish"

	if !webhook.IsDestination(destination) {
		return messaging.NonRetryable(fmt.Errorf("%s: destination is not a merchant webhook destination", op))
	}

	eventType := string(msg.Headers["event_type"])
	logger := p.logger.With(
		"destination",
		destination,
		"event_type",
		eventType,
		"outcome",
		OutcomeSkipped,
	)
	logger.Info("webhook delivery skipped because noop provider is enabled")
	p.businessMetrics.WebhookDeliveryCompleted(ctx, OutcomeSkipped)

	return messaging.Skipped(fmt.Errorf("%s: webhook noop provider is enabled", op))
}

// Close releases no resources.
func (p *Publisher) Close() error { return nil }
