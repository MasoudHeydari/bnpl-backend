package contractuc

import (
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ErrPaymentRequiresReconciliation indicates Financial Core reported a payment that
// BNPL cannot automatically apply and must be reconciled manually.
var ErrPaymentRequiresReconciliation = errors.New("payment requires reconciliation")

// ReconciliationError carries a stable reconciliation reason for Financial Core payments
// that cannot be applied automatically.
type ReconciliationError struct {
	// Reason describes why payment reconciliation failed.
	Reason model.PaymentReconciliationReason
	// Err is the underlying wrapped error.
	Err error
}

// Error implements the error interface.
func (e *ReconciliationError) Error() string {
	if e.Err == nil {
		return string(e.Reason)
	}

	return fmt.Sprintf("%s: %v", e.Reason, e.Err)
}

// Unwrap returns the underlying error cause.
func (e *ReconciliationError) Unwrap() error {
	return e.Err
}

// ReconciliationOutput reports a durable reconciliation outcome.
type ReconciliationOutput struct {
	// Result reports the use-case outcome classification.
	Result ActivateResult

	// Reason describes why the error or outcome occurred.
	Reason model.PaymentReconciliationReason
}

func newReconciliationError(
	reason model.PaymentReconciliationReason,
	err error,
) *ReconciliationError {
	return &ReconciliationError{
		Reason: reason,
		Err:    err,
	}
}
