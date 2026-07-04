package sarama

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/IBM/sarama"

	adaptermessaging "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

// ErrFatalConsumerExit indicates the Kafka consumer exited with a fatal error.
var ErrFatalConsumerExit = errors.New("fatal kafka consumer exit")

// ConsumerConfig configures Kafka consumer behavior.
type ConsumerConfig struct {
	InitialOffset       adaptermessaging.InitialOffset
	RetryInitialBackoff time.Duration
	RetryMaxBackoff     time.Duration
	ConsumerHealth      corehealth.ConsumerStateWriter

	// QuarantineTopic receives transport/envelope messages explicitly marked
	// non-retryable by a handler. The source offset is acknowledged only after
	// the quarantine publish succeeds.
	QuarantineTopic string

	// QuarantinePublisher is shared with the normal application publisher. The
	// consumer borrows it and must not close it.
	QuarantinePublisher messaging.Publisher
}

var _ messaging.Subscriber = (*Consumer)(nil)

// Consumer consumes Kafka topics through a Sarama consumer group.
type Consumer struct {
	group   sarama.ConsumerGroup
	groupID string
	topics  []string
	logger  log.Logger
	cfg     ConsumerConfig
}

// NewConsumer creates a Kafka consumer group subscriber for the configured topics.
func NewConsumer(
	brokers []string,
	groupID string,
	topics []string,
	logger log.Logger,
	cfg ConsumerConfig,
) (*Consumer, error) {
	const op = "sarama.NewConsumer"

	if len(brokers) == 0 {
		return nil, fmt.Errorf("%s: kafka consumer brokers: empty", op)
	}

	if groupID == "" {
		return nil, fmt.Errorf("%s: kafka consumer group_id: empty", op)
	}

	if len(topics) == 0 {
		return nil, fmt.Errorf("%s: kafka consumer topics: empty", op)
	}

	if logger == nil {
		return nil, fmt.Errorf("%s: kafka consumer logger is nil", op)
	}

	if cfg.RetryInitialBackoff <= 0 {
		return nil, fmt.Errorf("%s: kafka consumer retry initial backoff must be positive", op)
	}

	if cfg.RetryMaxBackoff < cfg.RetryInitialBackoff {
		return nil, fmt.Errorf("%s: kafka consumer retry max backoff must be greater than or equal to initial backoff", op)
	}

	if cfg.QuarantineTopic == "" {
		return nil, fmt.Errorf("%s: kafka consumer quarantine topic: empty", op)
	}

	if cfg.QuarantinePublisher == nil {
		return nil, fmt.Errorf("%s: kafka consumer quarantine publisher: nil", op)
	}

	initialOffset, err := mapInitialOffset(cfg.InitialOffset)
	if err != nil {
		return nil, err
	}

	config := sarama.NewConfig()
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	config.Consumer.Offsets.Initial = initialOffset

	group, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		return nil, fmt.Errorf("%s: create kafka consumer group: %w", op, err)
	}

	return &Consumer{group: group, groupID: groupID, topics: topics, logger: logger, cfg: cfg}, nil
}

func mapInitialOffset(value adaptermessaging.InitialOffset) (int64, error) {
	const op = "sarama.mapInitialOffset"

	switch value {
	case adaptermessaging.InitialOffsetOldest:
		return sarama.OffsetOldest, nil

	case adaptermessaging.InitialOffsetNewest:
		return sarama.OffsetNewest, nil

	default:
		return 0, fmt.Errorf("%s: unsupported initial offset %q", op, value)
	}
}

// Subscribe runs the consumer loop until ctx is canceled or a fatal error occurs.
func (c *Consumer) Subscribe(ctx context.Context, handler messaging.Handler) error {
	const op = "sarama.Consumer.Subscribe"

	if handler == nil {
		return fmt.Errorf("%s: kafka consumer handler: nil", op)
	}

	consumerHandler := &consumerGroupHandler{
		handler:             handler,
		logger:              c.logger,
		groupID:             c.groupID,
		quarantineTopic:     c.cfg.QuarantineTopic,
		quarantinePublisher: c.cfg.QuarantinePublisher,
		onSessionStart: func() {
			c.setConsumerState(corehealth.ConsumerHealthy)
		},
	}

	c.setConsumerState(corehealth.ConsumerStarting)

	backoff := c.cfg.RetryInitialBackoff
	retries := 0

	for {
		if err := ctx.Err(); err != nil {
			c.setConsumerState(corehealth.ConsumerStopped)
			return err
		}

		err := c.group.Consume(ctx, c.topics, consumerHandler)
		if err == nil {
			backoff = c.cfg.RetryInitialBackoff
			retries = 0
			continue
		}

		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			c.setConsumerState(corehealth.ConsumerStopped)
			return context.Canceled
		}

		if isFatalConsumeError(err) {
			c.setConsumerState(corehealth.ConsumerFatal)
			return fmt.Errorf("%s: %w: %w", op, ErrFatalConsumerExit, err)
		}

		c.setConsumerState(corehealth.ConsumerRetrying)
		retries++

		c.logger.With(
			"error", err.Error(),
			"retry_count", retries,
			"backoff", backoff.String(),
		).Warn("kafka consumer session failed; retrying")

		if waitErr := waitWithContext(ctx, jitteredBackoff(backoff)); waitErr != nil {
			c.setConsumerState(corehealth.ConsumerStopped)
			return waitErr
		}

		backoff = nextBackoff(backoff, c.cfg.RetryMaxBackoff)
	}
}

func (c *Consumer) setConsumerState(state corehealth.ConsumerRuntimeState) {
	if c.cfg.ConsumerHealth == nil {
		return
	}

	switch state {
	case corehealth.ConsumerStarting:
		c.cfg.ConsumerHealth.SetStarting()

	case corehealth.ConsumerHealthy:
		c.cfg.ConsumerHealth.SetHealthy()

	case corehealth.ConsumerRetrying:
		c.cfg.ConsumerHealth.SetRetrying()

	case corehealth.ConsumerFatal:
		c.cfg.ConsumerHealth.SetFatal()

	case corehealth.ConsumerStopped:
		c.cfg.ConsumerHealth.SetStopped()
	}
}

func isFatalConsumeError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, sarama.ErrClosedClient) || errors.Is(err, sarama.ErrClosedConsumerGroup) {
		return true
	}

	var configErr sarama.ConfigurationError
	if errors.As(err, &configErr) {
		return true
	}

	var kErr sarama.KError
	if errors.As(err, &kErr) {
		switch {
		case errors.Is(kErr, sarama.ErrTopicAuthorizationFailed),
			errors.Is(kErr, sarama.ErrGroupAuthorizationFailed),
			errors.Is(kErr, sarama.ErrClusterAuthorizationFailed),
			errors.Is(kErr, sarama.ErrTransactionalIDAuthorizationFailed),
			errors.Is(kErr, sarama.ErrDelegationTokenAuthorizationFailed),
			errors.Is(kErr, sarama.ErrSASLAuthenticationFailed),
			errors.Is(kErr, sarama.ErrUnsupportedSASLMechanism),
			errors.Is(kErr, sarama.ErrIllegalSASLState),
			errors.Is(kErr, sarama.ErrUnsupportedVersion),
			errors.Is(kErr, sarama.ErrInvalidConfig),
			errors.Is(kErr, sarama.ErrSecurityDisabled):
			return true

		default:
			return false
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	return false
}

func nextBackoff(current, max time.Duration) time.Duration {
	if current >= max || current > max/2 {
		return max
	}

	return current * 2
}

func jitteredBackoff(base time.Duration) time.Duration {
	if base <= 0 {
		return base
	}

	jitterWindow := base / 5
	if jitterWindow <= 0 {
		return base
	}

	jitter := time.Duration(rand.Int63n(int64(jitterWindow)))
	if jitter == 0 {
		jitter = time.Nanosecond
	}

	const maxDuration = time.Duration(1<<63 - 1)
	if base > maxDuration-jitter {
		return maxDuration
	}

	return base + jitter
}

func waitWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-timer.C:
		return nil
	}
}

// Close shuts down the Kafka consumer group and marks the consumer stopped.
func (c *Consumer) Close() error {
	const op = "sarama.Consumer.Close"

	if err := c.group.Close(); err != nil {
		return fmt.Errorf("%s: close kafka consumer group: %w", op, err)
	}

	c.setConsumerState(corehealth.ConsumerStopped)

	return nil
}

type consumerGroupHandler struct {
	handler             messaging.Handler
	logger              log.Logger
	groupID             string
	quarantineTopic     string
	quarantinePublisher messaging.Publisher
	onSessionStart      func()
}

func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	if h.onSessionStart != nil {
		h.onSessionStart()
	}

	return nil
}

func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(
	session sarama.ConsumerGroupSession,
	claim sarama.ConsumerGroupClaim,
) error {
	for msg := range claim.Messages() {
		coreMsg := toMessage(msg)

		if err := h.handler(session.Context(), coreMsg); err != nil {
			if messaging.IsNonRetryable(err) {
				h.logError("kafka.message.non_retryable", msg, err)

				// Quarantine must become durable before the source offset is
				// acknowledged. If this publish fails, return the error so Kafka
				// redelivers the original message instead of silently losing it.
				if quarantineErr := publishQuarantine(
					session.Context(),
					h.quarantinePublisher,
					h.quarantineTopic,
					h.groupID,
					msg,
					err,
				); quarantineErr != nil {
					h.logError("kafka.message.quarantine_failed", msg, quarantineErr)
					return quarantineErr
				}

				session.MarkMessage(msg, "quarantined")
				continue
			}

			h.logError("kafka.message.retryable_failed", msg, err)
			return err
		}

		session.MarkMessage(msg, "")
	}

	return nil
}

func (h *consumerGroupHandler) logError(event string, msg *sarama.ConsumerMessage, err error) {
	h.logger.Error(
		event,
		"topic", msg.Topic,
		"partition", msg.Partition,
		"offset", msg.Offset,
		"error", err.Error(),
	)
}

func toMessage(msg *sarama.ConsumerMessage) messaging.Message {
	headers := make(map[string][]byte, len(msg.Headers))
	for _, h := range msg.Headers {
		headers[string(h.Key)] = h.Value
	}

	return messaging.Message{
		Key:       msg.Key,
		Value:     msg.Value,
		Topic:     msg.Topic,
		Partition: msg.Partition,
		Offset:    msg.Offset,
		Timestamp: msg.Timestamp,
		Headers:   headers,
	}
}
