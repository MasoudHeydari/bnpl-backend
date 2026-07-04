package financialcorepaymentresultuc

import (
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ReconciliationApplicationError identifies a deterministic financial inconsistency
// found while applying an authoritative reconciliation success. Callers must roll
// back the application transaction before persisting the durable reconciliation
// outcome in a separate transaction.
type ReconciliationApplicationError struct {
	// Reason describes why reconciliation application failed.
	Reason model.PaymentReconciliationReason
	// Err is the underlying wrapped error.
	Err error
}

// Error implements error.
func (e *ReconciliationApplicationError) Error() string {
	if e == nil {
		return ""
	}

	if e.Err == nil {
		return fmt.Sprintf("reconciliation application conflict: %s", e.Reason)
	}

	return fmt.Sprintf("reconciliation application conflict (%s): %v", e.Reason, e.Err)
}

// Unwrap returns the underlying deterministic error.
func (e *ReconciliationApplicationError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

func newReconciliationApplicationError(
	reason model.PaymentReconciliationReason,
	err error,
) error {
	return &ReconciliationApplicationError{Reason: reason, Err: err}
}

// AsReconciliationApplicationError extracts a deterministic reconciliation
// application error without relying on string matching.
func AsReconciliationApplicationError(
	err error,
) (*ReconciliationApplicationError, bool) {
	var target *ReconciliationApplicationError
	if !errors.As(err, &target) {
		return nil, false
	}

	return target, true
}

func wrapDeterministicReconciliationApplicationError(
	reason model.PaymentReconciliationReason,
	err error,
) error {
	if err == nil {
		return nil
	}

	if cerr.IsCode(err, cerr.CodeConflict) ||
		cerr.IsCode(err, cerr.CodeValidationFailed) ||
		cerr.IsCode(err, cerr.CodeIdempotencyConflict) ||
		cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return newReconciliationApplicationError(reason, err)
	}

	return err
}
