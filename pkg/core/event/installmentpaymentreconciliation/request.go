package installmentpaymentreconciliation

import (
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	// ReconciliationRequestedEventType is the outbox event type for reconciliation requests.
	ReconciliationRequestedEventType = "bnpl.installment_payment_reconciliation_requested.v1"

	// ReconciliationRequestedEventVersion is the envelope version for reconciliation requests.
	ReconciliationRequestedEventVersion = "1"

	// ProducerName identifies BNPL as the producer of reconciliation requests.
	ProducerName = "bnpl-backend"
)

// RequestEvent is the outbound reconciliation request envelope.
type RequestEvent struct {
	// EventID uniquely identifies this reconciliation request.
	EventID string `json:"event_id"`
	// EventType is the machine-readable outbox event type.
	EventType string `json:"event_type"`
	// EventVersion is the envelope schema version.
	EventVersion string `json:"event_version"`
	// Producer identifies the service that produced the event.
	Producer string `json:"producer"`
	// OccurredAt is when the reconciliation request was created.
	OccurredAt time.Time `json:"occurred_at"`

	// Data carries the reconciliation request payload.
	Data RequestEventData `json:"data"`
}

// RequestEventData carries the reconciliation request payload.
type RequestEventData struct {
	// BNPLPaymentAttemptID is the BNPL payment attempt requiring reconciliation.
	BNPLPaymentAttemptID model.PaymentAttemptID `json:"bnpl_payment_attempt_id"`
	// ContractID is the BNPL contract associated with the payment attempt.
	ContractID model.ContractID `json:"contract_id"`
	// InstallmentID is the BNPL installment associated with the payment attempt.
	InstallmentID model.InstallmentID `json:"installment_id"`
	// CustomerID is the internal BNPL customer primary key for the attempt.
	CustomerID model.CustomerID `json:"customer_id"`

	// FinancialPaymentIntentID is the Financial Core payment intent, when known.
	FinancialPaymentIntentID *string `json:"financial_payment_intent_id,omitempty"`
	// FinancialPaymentSessionID is the Financial Core payment session, when known.
	FinancialPaymentSessionID *string `json:"financial_payment_session_id,omitempty"`

	// AmountMinor is the attempted payment amount in minor currency units.
	AmountMinor uint64 `json:"amount_minor"`
	// Currency is the currency of AmountMinor.
	Currency model.Currency `json:"currency"`

	// CurrentLocalState is BNPL's current local payment-attempt state.
	CurrentLocalState model.PaymentAttemptState `json:"current_local_state"`
	// ReconciliationReason explains why reconciliation was requested.
	ReconciliationReason model.PaymentReconciliationReason `json:"reconciliation_reason"`
	// ReconciliationGeneration is the monotonic generation for this reconciliation cycle.
	ReconciliationGeneration int `json:"reconciliation_generation"`

	// RequestedAt is when BNPL requested reconciliation.
	RequestedAt time.Time `json:"requested_at"`
}

// NewRequestEvent constructs a reconciliation request envelope.
func NewRequestEvent(eventID string, occurredAt time.Time, data RequestEventData) RequestEvent {
	return RequestEvent{
		EventID:      eventID,
		EventType:    ReconciliationRequestedEventType,
		EventVersion: ReconciliationRequestedEventVersion,
		Producer:     ProducerName,
		OccurredAt:   occurredAt.UTC(),
		Data:         data,
	}
}

// ValidateRequestEvent validates a reconciliation request before outbox insertion.
func ValidateRequestEvent(event RequestEvent) error {
	const op = "installmentpaymentreconciliation.ValidateRequestEvent"

	data := event.Data

	if data.BNPLPaymentAttemptID.IsZero() {
		return errors.New("bnpl payment attempt id is required")
	}

	if data.ContractID.IsZero() {
		return errors.New("contract id is required")
	}

	if data.InstallmentID.IsZero() {
		return errors.New("installment id is required")
	}

	if data.CustomerID.IsZero() {
		return errors.New("customer profile id is required")
	}

	if data.AmountMinor == 0 {
		return errors.New("amount minor is required")
	}

	if data.Currency == "" {
		return errors.New("currency is required")
	}

	if !data.CurrentLocalState.IsValid() {
		return errors.New("current local state is invalid")
	}

	if !data.ReconciliationReason.IsValid() {
		return errors.New("reconciliation reason is invalid")
	}

	if data.ReconciliationGeneration <= 0 {
		return fmt.Errorf("%s: reconciliation generation must be positive", op)
	}

	if data.RequestedAt.IsZero() {
		return errors.New("requested at is required")
	}

	return nil
}

// RequestEventID returns the deterministic reconciliation request event id for a payment attempt generation.
func RequestEventID(attemptID model.PaymentAttemptID, generation int) string {
	return fmt.Sprintf("%s:reconciliation:%d", attemptID, generation)
}
