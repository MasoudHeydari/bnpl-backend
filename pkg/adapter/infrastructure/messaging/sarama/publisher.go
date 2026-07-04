package sarama

import (
	"context"
	"fmt"
	"time"

	"github.com/IBM/sarama"

	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

var _ messaging.Publisher = (*Producer)(nil)

// Producer publishes messages through a Sarama sync producer.
type Producer struct {
	producer sarama.SyncProducer
}

// NewProducer creates a Kafka sync producer configured for reliable delivery.
func NewProducer(
	brokers []string,
	returnSuccesses bool,
	requiredAcks sarama.RequiredAcks,
	maxRetries int,
) (*Producer, error) {
	const op = "sarama.NewProducer"

	if len(brokers) == 0 {
		return nil, fmt.Errorf("%s: kafka producer brokers: empty", op)
	}

	config := sarama.NewConfig()
	config.Producer.Return.Successes = returnSuccesses
	config.Producer.RequiredAcks = requiredAcks
	config.Producer.Retry.Max = maxRetries
	config.Producer.Timeout = 10 * time.Second

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("%s: create kafka producer: %w", op, err)
	}

	return &Producer{producer: producer}, nil
}

// Publish sends a message to the given Kafka topic.
func (p *Producer) Publish(ctx context.Context, topic string, msg messaging.Message) error {
	const op = "sarama.Producer.Publish"

	if topic == "" {
		return fmt.Errorf("%s: kafka publish topic: empty", op)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	default:
	}

	kafkaMsg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.ByteEncoder(msg.Key),
		Value: sarama.ByteEncoder(msg.Value),
	}

	if len(msg.Headers) > 0 {
		kafkaMsg.Headers = make([]sarama.RecordHeader, 0, len(msg.Headers))
		for key, value := range msg.Headers {
			kafkaMsg.Headers = append(kafkaMsg.Headers, sarama.RecordHeader{
				Key:   []byte(key),
				Value: value,
			})
		}
	}

	_, _, err := p.producer.SendMessage(kafkaMsg)
	if err != nil {
		return fmt.Errorf("%s: publish kafka message to %q: %w", op, topic, err)
	}

	return nil
}

// Close shuts down the underlying Kafka producer.
func (p *Producer) Close() error {
	const op = "sarama.Producer.Close"

	if err := p.producer.Close(); err != nil {
		return fmt.Errorf("%s: close kafka producer: %w", op, err)
	}

	return nil
}
