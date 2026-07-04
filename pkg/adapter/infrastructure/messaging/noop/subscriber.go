package noop

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/health"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

// Verify Subscriber implements the core messaging subscriber contract.
var _ messaging.Subscriber = (*Subscriber)(nil)

// Subscriber is a no-op message subscriber.
//
// Subscribe blocks until the context is canceled and never invokes the
// provided handler because there is no backing broker.
type Subscriber struct {
	consumerHealth health.ConsumerStateWriter
}

// NewSubscriber constructs a no-op messaging subscriber.
func NewSubscriber() messaging.Subscriber {
	return &Subscriber{}
}

// NewSubscriberWithHealth constructs a no-op subscriber that reports its runtime state.
func NewSubscriberWithHealth(consumerHealth health.ConsumerStateWriter) messaging.Subscriber {
	return &Subscriber{consumerHealth: consumerHealth}
}

// Subscribe waits until ctx is canceled.
//
// The handler is intentionally ignored because noop messaging never receives
// messages.
func (s *Subscriber) Subscribe(ctx context.Context, _ messaging.Handler) error {
	if s.consumerHealth != nil {
		s.consumerHealth.SetHealthy()
		defer s.consumerHealth.SetStopped()
	}

	<-ctx.Done()
	return ctx.Err()
}

// Close releases no resources and always succeeds.
func (s *Subscriber) Close() error {
	return nil
}
