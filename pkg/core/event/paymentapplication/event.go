package paymentapplication

import (
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	// PaymentApplicationResultEventType is the machine-readable outbox event type for
	// payment application acknowledgements sent to Financial Core consumers.
	PaymentApplicationResultEventType = "bnpl.payment_application_result.v1"

	// PaymentApplicationResultEventVersion is the envelope version for outbound acknowledgements.
	PaymentApplicationResultEventVersion = "1"

	// ProducerName identifies BNPL as the producer of payment-application acknowledgements.
	ProducerName = "bnpl-backend"
)

// Event is the outbound business acknowledgement envelope published through the transactional outbox.
//
// This is not a broker transport acknowledgement. Financial Core already owns monetary truth;
// this event reports whether BNPL applied that successful payment to its own obligation.
type Event struct {
	// EventID uniquely identifies this payment-application acknowledgement.
	EventID string `json:"event_id"`
	// EventType is the machine-readable outbox event type.
	EventType string `json:"event_type"`
	// EventVersion is the envelope schema version.
	EventVersion string `json:"event_version"`
	// Producer identifies the service that produced the event.
	Producer string `json:"producer"`
	// OccurredAt is when BNPL produced the acknowledgement.
	OccurredAt time.Time `json:"occurred_at"`

	// CorrelationID links this acknowledgement to a request or workflow correlation.
	CorrelationID *string `json:"correlation_id,omitempty"`
	// CausationEventID references the Financial Core payment-result event being acknowledged.
	CausationEventID string `json:"causation_event_id"`

	// Data carries the payment-application acknowledgement payload.
	Data EventData `json:"data"`
}

// EventData carries the payment-application acknowledgement payload.
type EventData struct {
	// FinancialPaymentIntentID is the Financial Core payment intent being acknowledged.
	FinancialPaymentIntentID string `json:"financial_payment_intent_id"`
	// FinancialPaymentAttemptID is the Financial Core payment attempt, when known.
	FinancialPaymentAttemptID *string `json:"financial_payment_attempt_id,omitempty"`

	// PaymentPurpose identifies the Financial Core payment purpose being applied.
	PaymentPurpose model.FinancialCorePaymentPurpose `json:"payment_purpose"`

	// ContractID is the BNPL contract affected by the application, when known.
	ContractID *model.ContractID `json:"contract_id,omitempty"`
	// InstallmentID is the BNPL installment affected by the application, when known.
	InstallmentID *model.InstallmentID `json:"installment_id,omitempty"`
	// BNPLPaymentAttemptID is the BNPL payment attempt correlated to the payment, when known.
	BNPLPaymentAttemptID *model.PaymentAttemptID `json:"bnpl_payment_attempt_id,omitempty"`

	// Result reports how BNPL applied the Financial Core payment.
	Result ApplicationResult `json:"result"`
	// Reason explains why reconciliation is required, when Result needs it.
	Reason *model.PaymentReconciliationReason `json:"reason,omitempty"`

	// PaidAmountMinor is the paid amount in minor units for successful applications.
	PaidAmountMinor *uint64 `json:"paid_amount_minor,omitempty"`
	// Currency is the currency of PaidAmountMinor, when present.
	Currency *model.Currency `json:"currency,omitempty"`

	// FinancialPaidAt is when Financial Core recorded the successful payment.
	FinancialPaidAt *time.Time `json:"financial_paid_at,omitempty"`
	// BNPLAppliedAt is when BNPL applied the payment to its obligation.
	BNPLAppliedAt *time.Time `json:"bnpl_applied_at,omitempty"`

	// ContractStatus is the resulting BNPL contract status after application, when known.
	ContractStatus *model.ContractStatus `json:"contract_status,omitempty"`
	// InstallmentStatus is the resulting BNPL installment status after application, when known.
	InstallmentStatus *model.InstallmentStatus `json:"installment_status,omitempty"`
}

// NewEvent constructs a payment-application acknowledgement envelope.
func NewEvent(
	eventID string,
	causationEventID string,
	correlationID *string,
	occurredAt time.Time,
	data EventData,
) Event {
	return Event{
		EventID:          eventID,
		EventType:        PaymentApplicationResultEventType,
		EventVersion:     PaymentApplicationResultEventVersion,
		Producer:         ProducerName,
		OccurredAt:       occurredAt.UTC(),
		CorrelationID:    correlationID,
		CausationEventID: causationEventID,
		Data:             data,
	}
}

// ValidateEvent validates a payment-application acknowledgement before outbox insertion.
func ValidateEvent(event Event) error {
	const op = "paymentapplication.ValidateEvent"

	if event.Data.FinancialPaymentIntentID == "" {
		return errors.New("financial payment intent id is required")
	}

	switch event.Data.Result {
	case ReconciliationRequiredApplicationResult:
		if event.Data.Reason == nil || !event.Data.Reason.IsValid() {
			return errors.New("reconciliation reason is required")
		}

		return nil

	case AppliedApplicationResult, AlreadyAppliedApplicationResult:
		if event.Data.PaidAmountMinor == nil || *event.Data.PaidAmountMinor == 0 {
			return fmt.Errorf("%s: paid amount minor is required for %q", op, event.Data.Result)
		}

		if event.Data.Currency == nil || *event.Data.Currency == "" {
			return fmt.Errorf("%s: currency is required for %q", op, event.Data.Result)
		}

		if event.Data.FinancialPaidAt == nil || event.Data.FinancialPaidAt.IsZero() {
			return fmt.Errorf("%s: financial paid at is required for %q", op, event.Data.Result)
		}

		if event.Data.Reason != nil {
			return fmt.Errorf("%s: reason must be absent for %q", op, event.Data.Result)
		}

		return nil

	default:
		return fmt.Errorf("%s: unsupported application result %q", op, event.Data.Result)
	}
}
