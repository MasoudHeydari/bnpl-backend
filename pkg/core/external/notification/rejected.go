package notification

import (
	"errors"
	"fmt"
)

// ErrRejected reports a definite Notification rejection of a submission.
// Callers may clear the matching OTP issuance and allow a later START to
// replace it.
//
// Ambiguous failures (timeouts, transport errors, ALREADY_EXISTS) must NOT be
// wrapped in Rejected: leaving the issuance in-flight prevents issuing a fresh
// code while the first attempt's outcome is unknown. The idempotency key is
// stable for the issuance. BNPL does not persist plaintext OTP and therefore
// does not resubmit the same issuance in a later START request.
var ErrRejected = errors.New("notification rejected")

// Rejected wraps err as a definite Notification rejection.
func Rejected(err error) error {
	if err == nil {
		return ErrRejected
	}

	return fmt.Errorf("%w: %w", ErrRejected, err)
}

// IsRejected reports whether err is (or wraps) a definite Notification rejection.
func IsRejected(err error) bool {
	return errors.Is(err, ErrRejected)
}
