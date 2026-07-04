package installmentpaymentreconciliation

import (
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	// ReconciliationResultEventType is the inbound Financial Core reconciliation result event type.
	ReconciliationResultEventType = "financial_core.installment_payment_reconciliation_result.v1"

	// ReconciliationResultEventVersion is the envelope version for reconciliation results.
	ReconciliationResultEventVersion = "1"

	// FinancialCoreSourceService identifies Financial Core as the producer of reconciliation results.
	FinancialCoreSourceService = "financial-core"
)

// ResultStatus is the typed BNPL interpretation of a Financial Core reconciliation result.
type ResultStatus string

// Known reconciliation result statuses from Financial Core.
const (
	PendingResultStatus   ResultStatus = "pending"
	SucceededResultStatus ResultStatus = "succeeded"
	FailedResultStatus    ResultStatus = "failed"
	CanceledResultStatus  ResultStatus = "canceled"
	NotFoundResultStatus  ResultStatus = "not_found"
)

// IsValid reports whether the reconciliation result status is supported.
func (s ResultStatus) IsValid() bool {
	switch s {
	case PendingResultStatus,
		SucceededResultStatus,
		FailedResultStatus,
		CanceledResultStatus,
		NotFoundResultStatus:
		return true

	default:
		return false
	}
}

// ResultEventData carries the inbound reconciliation result payload.
type ResultEventData struct {
	// RequestEventID references the outbound reconciliation request this result answers.
	RequestEventID string `json:"request_event_id"`
	// BNPLPaymentAttemptID is the BNPL payment attempt being reconciled.
	BNPLPaymentAttemptID model.PaymentAttemptID `json:"bnpl_payment_attempt_id"`
	// ReconciliationGeneration is the monotonic generation for this reconciliation cycle.
	ReconciliationGeneration int `json:"reconciliation_generation"`
	// Status is Financial Core's typed reconciliation result status.
	Status ResultStatus `json:"status"`
	// FinancialPaymentIntentID is the Financial Core payment intent, when known.
	FinancialPaymentIntentID *string `json:"financial_payment_intent_id,omitempty"`
	// FinancialPaymentAttemptID is the Financial Core payment attempt, when known.
	FinancialPaymentAttemptID *string `json:"financial_payment_attempt_id,omitempty"`
	// FinancialPaymentSessionID is the Financial Core payment session, when known.
	FinancialPaymentSessionID *string `json:"financial_payment_session_id,omitempty"`
	// PaymentURL is the payer URL for completing payment, when available.
	PaymentURL *string `json:"payment_url,omitempty"`
	// ExpiresAt is when the payment session expires, if known.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// PaidAt is when Financial Core recorded a successful payment, if known.
	PaidAt *time.Time `json:"paid_at,omitempty"`
	// FailureCode is a machine-readable failure code when Status indicates failure.
	FailureCode *string `json:"failure_code,omitempty"`
	// AmountMinor is the payment amount in minor currency units.
	AmountMinor uint64 `json:"amount_minor"`
	// Currency is the ISO currency code for AmountMinor.
	Currency string `json:"currency"`
	// OccurredAt is when Financial Core produced the reconciliation result.
	OccurredAt time.Time `json:"occurred_at"`
}

// ValidateResultEventData validates a reconciliation result payload.
func ValidateResultEventData(data ResultEventData) error {
	const op = "installmentpaymentreconciliation.ValidateResultEventData"

	if data.RequestEventID == "" {
		return errors.New("request event id is required")
	}

	if data.BNPLPaymentAttemptID.IsZero() {
		return errors.New("bnpl payment attempt id is required")
	}

	if data.ReconciliationGeneration <= 0 {
		return fmt.Errorf("%s: reconciliation generation must be positive", op)
	}

	if !data.Status.IsValid() {
		return fmt.Errorf("%s: unsupported reconciliation result status %q", op, data.Status)
	}

	if data.AmountMinor == 0 {
		return errors.New("amount minor is required")
	}

	if data.Currency == "" {
		return errors.New("currency is required")
	}

	if data.OccurredAt.IsZero() {
		return errors.New("occurred at is required")
	}

	return nil
}
