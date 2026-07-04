package botmessage

import (
	"errors"
	"fmt"
)

var (
	// ErrRetryableFailure marks a transport-neutral send failure for which
	// retrying is expected to be safe.
	ErrRetryableFailure = errors.New("bot message retryable failure")

	// ErrPermanentFailure marks a definitive send failure for which repeating
	// the same operation is not expected to succeed without changing input or
	// configuration.
	ErrPermanentFailure = errors.New("bot message permanent failure")

	// ErrAmbiguousFailure marks a send failure where the provider may have
	// accepted the message but BNPL could not observe the final outcome.
	// Blind retries may therefore create duplicate user-visible messages.
	ErrAmbiguousFailure = errors.New("bot message ambiguous failure")
)

// RetryableError wraps err with transport-neutral retry semantics.
//
// Adapters should use this only when replaying the same send is known to be
// safe, for example when a provider rejects the request with an explicitly
// temporary condition before accepting it.
func RetryableError(err error) error {
	const op = "botmessage.RetryableError"

	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w: %w", op, ErrRetryableFailure, err)
}

// PermanentError wraps err with transport-neutral permanent-failure semantics.
func PermanentError(err error) error {
	const op = "botmessage.PermanentError"

	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w: %w", op, ErrPermanentFailure, err)
}

// AmbiguousError wraps err when the external delivery outcome cannot be determined
// safely and a blind retry could duplicate the message.
func AmbiguousError(err error) error {
	const op = "botmessage.AmbiguousError"

	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w: %w", op, ErrAmbiguousFailure, err)
}
