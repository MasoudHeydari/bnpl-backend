package checkoutpayment

import "fmt"

// FailureClass is a transport-neutral classification of an external Financial
// Core failure. Idempotency (safe to repeat) is independent of whether a retry
// can change the outcome.
//
// The three axes modeled by helpers below are independent:
//
//  1. AllowsAutomaticRetry — may the same identity be retried automatically?
//  2. ExternalOutcomeKnown — is the remote monetary effect known from this error?
//  3. RequiresOperationalIntervention — dependency fix, reconciliation, or ops review?
type FailureClass string

const (
	// FailureClassTransient means a later attempt with the same identity can
	// succeed because the dependency was unavailable.
	FailureClassTransient FailureClass = "transient"

	// FailureClassAmbiguous means the remote effect may already have committed
	// and must be looked up or replayed with the same idempotency identity.
	// Covers DeadlineExceeded, gRPC Canceled, Unknown, and Internal.
	FailureClassAmbiguous FailureClass = "ambiguous"

	// FailureClassPermanentInvalidInput means the request is deterministically
	// rejected (InvalidArgument / OutOfRange) and repeating the same input
	// will not succeed.
	FailureClassPermanentInvalidInput FailureClass = "permanent_invalid_input"

	// FailureClassPermanentFailedPrecondition means a required remote
	// precondition is not met. It is not the same as invalid input and must
	// not be auto-retried.
	FailureClassPermanentFailedPrecondition FailureClass = "permanent_failed_precondition"

	// FailureClassPermanentAuth means authentication or authorization failed
	// (Unauthenticated / PermissionDenied). Automatic retry cannot help.
	FailureClassPermanentAuth FailureClass = "permanent_auth"

	// FailureClassPermanentConflict means stored financial identity or economics
	// disagree (AlreadyExists) and must not be overwritten by another attempt
	// with a newly generated identity.
	FailureClassPermanentConflict FailureClass = "permanent_conflict"

	// FailureClassRetryableConcurrency means a race with another worker can
	// converge if the operation is repeated with the same identity.
	FailureClassRetryableConcurrency FailureClass = "retryable_concurrency"

	// FailureClassNotFound means the referenced resource was not visible to the
	// caller. After a timeout this does NOT prove the prior write never
	// committed; treat the external outcome as unknown.
	FailureClassNotFound FailureClass = "not_found"

	// FailureClassUnknown means the failure has no FC-contracted recovery path
	// and must not automatically release financial gates.
	FailureClassUnknown FailureClass = "unknown"
)

// AllowsAutomaticRetry reports whether a later attempt with the same identity
// may be scheduled automatically (subject to retry budget).
func (c FailureClass) AllowsAutomaticRetry() bool {
	switch c {
	case FailureClassTransient,
		FailureClassAmbiguous,
		FailureClassRetryableConcurrency:
		return true

	default:
		return false
	}
}

// ExternalOutcomeKnown reports whether this failure alone proves the remote
// monetary effect. Transport failures never prove absence or success.
func (c FailureClass) ExternalOutcomeKnown() bool {
	return false
}

// RequiresOperationalIntervention reports whether dependency repair,
// reconciliation, or operator review is required before automatic progress.
func (c FailureClass) RequiresOperationalIntervention() bool {
	switch c {
	case FailureClassPermanentInvalidInput,
		FailureClassPermanentFailedPrecondition,
		FailureClassPermanentAuth,
		FailureClassPermanentConflict,
		FailureClassNotFound,
		FailureClassUnknown:
		return true

	default:
		return false
	}
}

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
