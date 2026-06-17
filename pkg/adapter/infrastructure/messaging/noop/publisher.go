package noop

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

// Verify Publisher implements the core messaging publisher contract.
var _ messaging.Publisher = (*Publisher)(nil)

// Publisher is a no-op message publisher.
//
// Publish succeeds without sending data anywhere unless the context has already been canceled.
type Publisher struct{}

// NewPublisher constructs a no-op messaging publisher.
func NewPublisher() messaging.Publisher {
	return &Publisher{}
}

// Publish accepts payload for topic and performs no broker operation.
func (p *Publisher) Publish(ctx context.Context, _ string, _ messaging.Message) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// Close releases no resources and always succeeds.
func (p *Publisher) Close() error {
	return nil
}
