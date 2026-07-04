package sarama

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/IBM/sarama"

	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

const maxQuarantineErrorBytes = 2048

// quarantineEnvelope is an operations-focused copy of a message that could not
// enter normal application processing. []byte fields are JSON base64-encoded by
// encoding/json so the exact transport bytes remain available for forensics.
type quarantineEnvelope struct {
	QuarantineID  string            `json:"quarantine_id"`
	Reason        string            `json:"reason"`
	ConsumerGroup string            `json:"consumer_group"`
	Source        quarantineSource  `json:"source"`
	Message       quarantineMessage `json:"message"`
	Failure       quarantineFailure `json:"failure"`
	QuarantinedAt time.Time         `json:"quarantined_at"`
}

type quarantineSource struct {
	Topic     string    `json:"topic"`
	Partition int32     `json:"partition"`
	Offset    int64     `json:"offset"`
	Timestamp time.Time `json:"timestamp"`
}

type quarantineMessage struct {
	Key     []byte            `json:"key,omitempty"`
	Value   []byte            `json:"value"`
	Headers map[string][]byte `json:"headers,omitempty"`
}

type quarantineFailure struct {
	Class string `json:"class"`
	Error string `json:"error"`
}

func publishQuarantine(
	ctx context.Context,
	publisher messaging.Publisher,
	topic string,
	groupID string,
	msg *sarama.ConsumerMessage,
	cause error,
) error {
	const op = "sarama.publishQuarantine"

	if publisher == nil {
		return fmt.Errorf("%s: quarantine publisher is nil", op)
	}

	if topic == "" {
		return fmt.Errorf("%s: quarantine topic is empty", op)
	}

	headers := make(map[string][]byte, len(msg.Headers))
	for _, header := range msg.Headers {
		headers[string(header.Key)] = append([]byte(nil), header.Value...)
	}

	envelope := quarantineEnvelope{
		QuarantineID:  quarantineID(groupID, msg.Topic, msg.Partition, msg.Offset),
		Reason:        "NON_RETRYABLE_HANDLER_ERROR",
		ConsumerGroup: groupID,
		Source: quarantineSource{
			Topic:     msg.Topic,
			Partition: msg.Partition,
			Offset:    msg.Offset,
			Timestamp: msg.Timestamp.UTC(),
		},
		Message: quarantineMessage{
			Key:     append([]byte(nil), msg.Key...),
			Value:   append([]byte(nil), msg.Value...),
			Headers: headers,
		},
		Failure: quarantineFailure{
			Class: "NON_RETRYABLE",
			Error: boundedError(cause),
		},
		QuarantinedAt: time.Now().UTC(),
	}

	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("%s: marshal quarantine envelope: %w", op, err)
	}

	return publisher.Publish(
		ctx,
		topic,
		messaging.Message{
			Key:   []byte(envelope.QuarantineID),
			Value: payload,
			Headers: map[string][]byte{
				"bnpl-quarantine-version": []byte("v1"),
				"bnpl-source-topic":       []byte(msg.Topic),
			},
		},
	)
}

func quarantineID(groupID, topic string, partition int32, offset int64) string {
	h := sha256.New()
	_, _ = h.Write([]byte(groupID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(topic))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.FormatInt(int64(partition), 10)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.FormatInt(offset, 10)))
	return "q_" + hex.EncodeToString(h.Sum(nil))
}

func boundedError(err error) string {
	if err == nil {
		return ""
	}

	value := err.Error()
	if len(value) <= maxQuarantineErrorBytes {
		return value
	}

	return value[:maxQuarantineErrorBytes]
}
