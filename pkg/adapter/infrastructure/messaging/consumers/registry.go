package consumers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

// ErrHandlerPanicked identifies a recovered panic from a registered message
// handler. It is intentionally retryable so the message is not acknowledged as
// successfully processed after an unexpected application failure.
var ErrHandlerPanicked = errors.New("message consumer handler panicked")

// Registry routes consumed messages to topic-specific handlers.
type Registry struct {
	logger   log.Logger
	reporter coreerrorreporting.Reporter
	handlers map[string]messaging.Handler
}

// NewRegistry creates an empty consumer registry.
func NewRegistry(logger log.Logger, reporter coreerrorreporting.Reporter) (*Registry, error) {
	const op = "consumers.NewRegistry"

	if logger == nil {
		return nil, fmt.Errorf("%s: consumer registry logger is nil", op)
	}

	if reporter == nil {
		return nil, fmt.Errorf("%s: consumer registry reporter is nil", op)
	}

	return &Registry{
		logger:   logger,
		reporter: reporter,
		handlers: make(map[string]messaging.Handler),
	}, nil
}

// Register binds a handler to a Kafka topic.
func (r *Registry) Register(topic string, handler messaging.Handler) error {
	const op = "consumers.Registry.Register"

	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("%s: consumer topic is required", op)
	}

	if handler == nil {
		return fmt.Errorf("%s: consumer handler is required for topic %q", op, topic)
	}

	if _, exists := r.handlers[topic]; exists {
		return fmt.Errorf("%s: consumer handler already registered for topic %q", op, topic)
	}

	r.handlers[topic] = handler

	return nil
}

// Handle routes msg to the registered handler for its topic.
//
// The registry is the application callback boundary for consumed messages. A
// handler panic is recovered here, reported, and returned as a retryable error
// so the subscriber does not acknowledge the message and the process remains
// alive.
func (r *Registry) Handle(ctx context.Context, msg messaging.Message) (err error) {
	const op = "consumers.Registry.Handle"

	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}

		metadata := coreerrorreporting.Metadata{
			Component: "messaging_consumer",
			Tags: map[string]string{
				"messaging.system": "kafka",
				"messaging.topic":  msg.Topic,
			},
			Contexts: map[string]map[string]any{
				"message": {
					"partition": msg.Partition,
					"offset":    msg.Offset,
				},
			},
			Fingerprint: []string{"messaging_consumer", "panic", msg.Topic},
		}

		r.reporter.CapturePanic(ctx, recovered, metadata)
		r.logger.With(
			"topic",
			msg.Topic,
			"partition",
			msg.Partition,
			"offset",
			msg.Offset,
			"panic",
			fmt.Sprint(recovered),
		).Error("message consumer panic recovered")

		err = fmt.Errorf("%s: %w: %v", op, ErrHandlerPanicked, recovered)
	}()

	if err := ctx.Err(); err != nil {
		return err
	}

	handler, ok := r.handlers[msg.Topic]
	if !ok {
		r.logger.With(
			"topic",
			msg.Topic,
			"partition",
			msg.Partition,
			"offset",
			msg.Offset,
		).Warn("received message without registered handler")

		return messaging.NonRetryable(fmt.Errorf("%s: unsupported message topic %q", op, msg.Topic))
	}

	return handler(ctx, msg)
}
