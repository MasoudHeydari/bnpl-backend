package sarama

import (
	"context"
	"fmt"

	"github.com/IBM/sarama"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

var _ messaging.Subscriber = (*Consumer)(nil)

type Consumer struct {
	group  sarama.ConsumerGroup
	topics []string
	logger log.Logger
}

func NewConsumer(
	brokers []string,
	groupID string,
	topics []string,
	logger log.Logger,
) (*Consumer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("kafka consumer brokers: empty")
	}
	if groupID == "" {
		return nil, fmt.Errorf("kafka consumer group_id: empty")
	}
	if len(topics) == 0 {
		return nil, fmt.Errorf("kafka consumer topics: empty")
	}

	config := sarama.NewConfig()
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	config.Consumer.Offsets.Initial = sarama.OffsetNewest

	group, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		return nil, fmt.Errorf("create kafka consumer group: %w", err)
	}

	return &Consumer{group: group, topics: topics, logger: logger}, nil
}

func (c *Consumer) Subscribe(ctx context.Context, handler messaging.Handler) error {
	if handler == nil {
		return fmt.Errorf("kafka consumer handler: nil")
	}

	consumerHandler := &consumerGroupHandler{
		handler: handler,
		logger:  c.logger,
	}

	for {
		if err := c.group.Consume(ctx, c.topics, consumerHandler); err != nil {
			return fmt.Errorf("consume kafka messages: %w", err)
		}

		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (c *Consumer) Close() error {
	if err := c.group.Close(); err != nil {
		return fmt.Errorf("close kafka consumer group: %w", err)
	}

	return nil
}

type consumerGroupHandler struct {
	handler messaging.Handler
	logger  log.Logger
}

func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
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
				session.MarkMessage(msg, "non-retryable")
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
	if h.logger == nil {
		return
	}

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
