package installmentpaymentreconciliation

import (
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	// ReconciliationAcknowledgedEventType is the outbox event type for reconciliation acknowledgements.
	ReconciliationAcknowledgedEventType = "bnpl.installment_payment_reconciliation_acknowledged.v1"

	// ReconciliationAcknowledgedEventVersion is the envelope version for acknowledgements.
	ReconciliationAcknowledgedEventVersion = "1"
)

// AcknowledgementResult identifies how BNPL processed a reconciliation result.
type AcknowledgementResult string

// Known acknowledgement results for installment payment reconciliation.
const (
	ResolvedSubmittedAcknowledgementResult           AcknowledgementResult = "resolved_submitted"
	ResolvedSucceededAcknowledgementResult           AcknowledgementResult = "resolved_succeeded"
	ResolvedFailedAcknowledgementResult              AcknowledgementResult = "resolved_failed"
	ResolvedCanceledAcknowledgementResult            AcknowledgementResult = "resolved_canceled"
	StillReconciliationRequiredAcknowledgementResult AcknowledgementResult = "still_reconciliation_required"
	AlreadyProcessedAcknowledgementResult            AcknowledgementResult = "already_processed"
	PermanentlyRejectedAcknowledgementResult         AcknowledgementResult = "permanently_rejected"
)

// AcknowledgementEvent is the outbound reconciliation acknowledgement envelope.
type AcknowledgementEvent struct {
	// EventID uniquely identifies this reconciliation acknowledgement.
	EventID string `json:"event_id"`
	// EventType is the machine-readable outbox event type.
	EventType string `json:"event_type"`
	// EventVersion is the envelope schema version.
	EventVersion string `json:"event_version"`
	// Producer identifies the service that produced the event.
	Producer string `json:"producer"`
	// OccurredAt is when the acknowledgement was created.
	OccurredAt time.Time `json:"occurred_at"`

	// Data carries the reconciliation acknowledgement payload.
	Data AcknowledgementEventData `json:"data"`
}

// AcknowledgementEventData carries the reconciliation acknowledgement payload.
type AcknowledgementEventData struct {
	// RequestEventID references the outbound reconciliation request being acknowledged.
	RequestEventID string `json:"request_event_id"`
	// FinancialCoreResultEventID references the inbound Financial Core result event.
	FinancialCoreResultEventID string `json:"financial_core_result_event_id"`

	// BNPLPaymentAttemptID is the BNPL payment attempt that was reconciled.
	BNPLPaymentAttemptID model.PaymentAttemptID `json:"bnpl_payment_attempt_id"`
	// ReconciliationGeneration is the monotonic generation for this reconciliation cycle.
	ReconciliationGeneration int `json:"reconciliation_generation"`

	// PreviousState is the payment-attempt state before processing the result.
	PreviousState model.PaymentAttemptState `json:"previous_state"`
	// CurrentState is the payment-attempt state after processing the result.
	CurrentState model.PaymentAttemptState `json:"current_state"`

	// Result reports how BNPL processed the reconciliation result.
	Result AcknowledgementResult `json:"result"`

	// FinancialPaymentIntentID is the Financial Core payment intent, when known.
	FinancialPaymentIntentID *string `json:"financial_payment_intent_id,omitempty"`
	// Reason explains why reconciliation remains required or was rejected, when applicable.
	Reason *model.PaymentReconciliationReason `json:"reason,omitempty"`

	// BNPLProcessedAt is when BNPL finished processing the reconciliation result.
	BNPLProcessedAt time.Time `json:"bnpl_processed_at"`
}

// NewAcknowledgementEvent constructs a reconciliation acknowledgement envelope.
func NewAcknowledgementEvent(eventID string, occurredAt time.Time, data AcknowledgementEventData) AcknowledgementEvent {
	return AcknowledgementEvent{
		EventID:      eventID,
		EventType:    ReconciliationAcknowledgedEventType,
		EventVersion: ReconciliationAcknowledgedEventVersion,
		Producer:     ProducerName,
		OccurredAt:   occurredAt.UTC(),
		Data:         data,
	}
}

// ValidateAcknowledgementEvent validates a reconciliation acknowledgement before outbox insertion.
func ValidateAcknowledgementEvent(event AcknowledgementEvent) error {
	const op = "installmentpaymentreconciliation.ValidateAcknowledgementEvent"

	data := event.Data

	if data.RequestEventID == "" {
		return errors.New("request event id is required")
	}

	if data.FinancialCoreResultEventID == "" {
		return errors.New("financial core result event id is required")
	}

	if data.BNPLPaymentAttemptID.IsZero() {
		return errors.New("bnpl payment attempt id is required")
	}

	if data.ReconciliationGeneration <= 0 {
		return fmt.Errorf("%s: reconciliation generation must be positive", op)
	}

	if !data.PreviousState.IsValid() || !data.CurrentState.IsValid() {
		return errors.New("payment attempt state is invalid")
	}

	switch data.Result {
	case ResolvedSubmittedAcknowledgementResult,
		ResolvedSucceededAcknowledgementResult,
		ResolvedFailedAcknowledgementResult,
		ResolvedCanceledAcknowledgementResult,
		StillReconciliationRequiredAcknowledgementResult,
		AlreadyProcessedAcknowledgementResult,
		PermanentlyRejectedAcknowledgementResult:

	default:
		return fmt.Errorf("%s: unsupported acknowledgement result %q", op, data.Result)
	}

	if data.BNPLProcessedAt.IsZero() {
		return errors.New("bnpl processed at is required")
	}

	if data.Reason != nil && !data.Reason.IsValid() {
		return errors.New("reason is invalid")
	}

	return nil
}
