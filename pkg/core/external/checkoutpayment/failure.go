package checkoutpayment

import "fmt"

// FailureClass is a transport-neutral classification of an external Financial
// Core failure. Idempotency (safe to repeat) is independent of whether a retry
// can change the outcome.
type FailureClass string

const (
	// FailureClassTransient means a later attempt with the same identity can
	// succeed because the dependency was unavailable.
	FailureClassTransient FailureClass = "transient"

	// FailureClassAmbiguous means the remote effect may already have committed
	// and must be replayed or reconciled with the same idempotency identity.
	FailureClassAmbiguous FailureClass = "ambiguous"

	// FailureClassPermanentInvalidInput means the request is deterministically
	// rejected and repeating the same input will not succeed.
	FailureClassPermanentInvalidInput FailureClass = "permanent_invalid_input"

	// FailureClassPermanentConflict means stored financial identity or economics
	// disagree and must not be overwritten by another attempt.
	FailureClassPermanentConflict FailureClass = "permanent_conflict"

	// FailureClassRetryableConcurrency means a race with another worker can
	// converge if the operation is repeated.
	FailureClassRetryableConcurrency FailureClass = "retryable_concurrency"

	// FailureClassUnknown means the failure has no safe automatic recovery.
	FailureClassUnknown FailureClass = "unknown"
)

// ClassifiedFailure carries a semantic Financial Core failure class.
//
// Adapters attach it at the transport boundary so core workflows can use
// errors.As without importing gRPC status codes.
type ClassifiedFailure struct {
	Class FailureClass
	Err   error
}

// NewFailure returns a classified external failure. A nil cause becomes a
// generic error so Error never panics.
func NewFailure(class FailureClass, err error) error {
	if err == nil {
		err = fmt.Errorf("financial core failure (%s)", class)
	}

	return &ClassifiedFailure{Class: class, Err: err}
}

// Error returns the underlying diagnostic text.
func (e *ClassifiedFailure) Error() string {
	if e == nil || e.Err == nil {
		return "financial core failure"
	}

	return e.Err.Error()
}

// Unwrap exposes the cause for errors.Is and errors.As.
func (e *ClassifiedFailure) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}
