package messaging

import (
	"context"
	"errors"
	"time"
)

// Message represents a transport-neutral message consumed from or published
// through messaging infrastructure.
//
// Delivery metadata is included for logging, tracing, debugging, and delivery
// workflows. Application code should treat these fields as transport metadata
// rather than domain data.
type Message struct {
	// Key is the optional routing key.
	Key []byte

	// Value is the raw message payload.
	Value []byte

	// Topic identifies the destination channel or stream name.
	Topic string

	// Partition is the delivery shard number when supported.
	Partition int32

	// Offset is the delivery position or sequence number when supported.
	Offset int64

	// Timestamp is the infrastructure-provided message timestamp.
	Timestamp time.Time

	// Headers contains transport metadata values.
	Headers map[string][]byte
}

// Decoder decodes raw message payloads into application DTOs.
//
// Serialization format is intentionally outside the core messaging contract.
// Adapters may provide JSON, protobuf, Avro, or other decoder implementations.
type Decoder interface {
	// Decode unmarshals data into v.
	Decode(data []byte, v any) error
}

// Handler processes a consumed message.
//
// Returning nil marks the message as successfully handled. Returning an error
// lets the subscriber decide whether the message should be retried, skipped, or
// routed to a dead-letter workflow.
type Handler func(ctx context.Context, msg Message) error

// Publisher publishes messages to the messaging infrastructure.
type Publisher interface {
	// Publish sends msg to the named destination.
	Publish(ctx context.Context, topic string, msg Message) error

	// Close releases publisher resources.
	Close() error
}

// Subscriber consumes messages from configured destinations.
//
// Concrete subscribers decide how delivery coordination, acknowledgements,
// retries, and shutdown are handled.
type Subscriber interface {
	// Subscribe starts consuming messages until ctx is canceled or a fatal
	// subscriber error occurs.
	Subscribe(ctx context.Context, handler Handler) error

	// Close releases subscriber resources.
	Close() error
}

// ErrNonRetryable marks a message handling error as not safe or useful to retry.
var ErrNonRetryable = errors.New("non-retryable message error")

// ErrSkipped marks an intentional non-delivery. Outbox publishers treat it as a
// terminal skip that must not be confused with a successful publish.
var ErrSkipped = errors.New("delivery skipped")

// NonRetryable wraps err so subscribers can detect that the message should not
// be retried.
//
// Passing nil returns nil.
func NonRetryable(err error) error {
	if err == nil {
		return nil
	}

	return errors.Join(ErrNonRetryable, err)
}

// IsNonRetryable reports whether err was marked with NonRetryable.
func IsNonRetryable(err error) bool {
	return errors.Is(err, ErrNonRetryable)
}

// Skipped wraps err so outbox publishers can mark the row skipped instead of
// published. Passing nil returns ErrSkipped alone.
func Skipped(err error) error {
	if err == nil {
		return ErrSkipped
	}

	return errors.Join(ErrSkipped, err)
}

// IsSkipped reports whether err was marked with Skipped.
func IsSkipped(err error) bool {
	return errors.Is(err, ErrSkipped)
}
