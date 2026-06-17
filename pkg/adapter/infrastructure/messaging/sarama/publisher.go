package sarama

import (
	"context"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

var _ messaging.Publisher = (*Producer)(nil)

type Producer struct {
	producer sarama.SyncProducer
}

func NewProducer(
	brokers []string,
	returnSuccesses bool,
	requiredAcks sarama.RequiredAcks,
	maxRetries int,
) (*Producer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("kafka producer brokers: empty")
	}

	config := sarama.NewConfig()
	config.Producer.Return.Successes = returnSuccesses
	config.Producer.RequiredAcks = requiredAcks
	config.Producer.Retry.Max = maxRetries
	config.Producer.Timeout = 10 * time.Second

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}

	return &Producer{producer: producer}, nil
}

func (p *Producer) Publish(ctx context.Context, topic string, msg messaging.Message) error {
	if topic == "" {
		return fmt.Errorf("kafka publish topic: empty")
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
		return fmt.Errorf("publish kafka message to %q: %w", topic, err)
	}

	return nil
}

func (p *Producer) Close() error {
	if err := p.producer.Close(); err != nil {
		return fmt.Errorf("close kafka producer: %w", err)
	}

	return nil
}
