package installmentpaymentrequest

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	// InstallmentPaymentRequestedEventType is the machine-readable outbox event type for
	// installment repayment requests sent to Financial Core.
	InstallmentPaymentRequestedEventType = "bnpl.installment_payment_requested.v1"

	// InstallmentPaymentRequestedEventVersion is the envelope version for outbound requests.
	InstallmentPaymentRequestedEventVersion = "1"

	// ProducerName identifies BNPL as the producer of installment payment requests.
	ProducerName = "bnpl-backend"

	// PaymentPurposeInstallment identifies installment repayment in Financial Core.
	PaymentPurposeInstallment = "installment"

	// SourceService identifies BNPL as the originating service.
	SourceService = "bnpl-backend"

	// SourceTypeBNPLPaymentAttempt identifies a BNPL payment attempt source reference.
	SourceTypeBNPLPaymentAttempt = "bnpl_payment_attempt"

	// SourceVersion is the current BNPL payment attempt source contract version.
	SourceVersion = int64(1)
)

// Event is the outbound installment payment request envelope published through the transactional outbox.
type Event struct {
	// EventID uniquely identifies this outbound installment payment request.
	EventID string `json:"event_id"`
	// EventType is the machine-readable outbox event type.
	EventType string `json:"event_type"`
	// EventVersion is the envelope schema version.
	EventVersion string `json:"event_version"`
	// Producer identifies the service that produced the event.
	Producer string `json:"producer"`
	// OccurredAt is when the payment request was created.
	OccurredAt time.Time `json:"occurred_at"`

	// Data carries the installment payment request payload.
	Data EventData `json:"data"`
}

// EventData carries the installment payment request payload.
type EventData struct {
	// BNPLPaymentAttemptID is the BNPL payment attempt that owns this request.
	BNPLPaymentAttemptID model.PaymentAttemptID `json:"bnpl_payment_attempt_id"`
	// ContractID is the BNPL contract being repaid.
	ContractID model.ContractID `json:"contract_id"`
	// InstallmentID is the BNPL installment being repaid.
	InstallmentID model.InstallmentID `json:"installment_id"`
	// CustomerID is the internal BNPL customer primary key for the repayment.
	CustomerID model.CustomerID `json:"customer_id"`
	// TenantID identifies the Financial Core tenant for the payment.
	TenantID string `json:"tenant_id"`
	// ContextID identifies the Financial Core request context for the payment.
	ContextID string `json:"context_id"`

	// AmountMinor is the repayment amount in minor currency units.
	AmountMinor uint64 `json:"amount_minor"`
	// Currency is the currency of AmountMinor.
	Currency model.Currency `json:"currency"`
	// Channel is the repayment channel selected for the payment session.
	Channel model.PaymentChannel `json:"channel"`
	// ReturnURL is an optional URL where the payer is redirected after payment.
	ReturnURL *string `json:"return_url,omitempty"`

	// PaymentPurpose identifies installment repayment in Financial Core.
	PaymentPurpose string `json:"payment_purpose"`

	// SourceService identifies the originating service that owns the source reference.
	SourceService string `json:"source_service"`
	// SourceType identifies the kind of source reference.
	SourceType string `json:"source_type"`
	// SourceID is the originating source reference identifier.
	SourceID string `json:"source_id"`
	// SourceVersion is the contract version of the source reference.
	SourceVersion int64 `json:"source_version"`

	// IdempotencyKey is the stable Financial Core idempotency identity for this request.
	IdempotencyKey string `json:"idempotency_key"`
	// RequestFingerprint is a hash of request material used to detect payload conflicts.
	RequestFingerprint string `json:"request_fingerprint"`

	// Items are the immutable payment-attempt obligation line snapshots.
	Items []EventItem `json:"items"`
}

// EventItem is an immutable payment-attempt obligation line snapshot.
type EventItem struct {
	// ItemType identifies the kind of payment-attempt obligation line.
	ItemType model.PaymentAttemptItemType `json:"item_type"`
	// SourceID identifies the domain object that produced this obligation line.
	SourceID uuid.UUID `json:"source_id"`
	// AmountMinor is the line amount in minor currency units.
	AmountMinor uint64 `json:"amount_minor"`
	// Effect describes how this line contributes to the payable total.
	Effect model.PaymentAttemptItemEffect `json:"effect"`
}

// NewEvent constructs an installment payment request envelope.
func NewEvent(eventID string, occurredAt time.Time, data EventData) Event {
	return Event{
		EventID:      eventID,
		EventType:    InstallmentPaymentRequestedEventType,
		EventVersion: InstallmentPaymentRequestedEventVersion,
		Producer:     ProducerName,
		OccurredAt:   occurredAt.UTC(),
		Data:         data,
	}
}

// EventItemsFromPaymentAttemptItems maps persisted attempt items to outbound event items.
func EventItemsFromPaymentAttemptItems(items []model.PaymentAttemptItem) []EventItem {
	if len(items) == 0 {
		return nil
	}

	out := make([]EventItem, 0, len(items))
	for _, item := range items {
		out = append(out, EventItem{
			ItemType:    item.ItemType,
			SourceID:    item.SourceID,
			AmountMinor: item.AmountMinor,
			Effect:      item.Effect,
		})
	}

	return out
}

// ValidateEvent validates an installment payment request before outbox insertion.
func ValidateEvent(event Event) error {
	const op = "installmentpaymentrequest.ValidateEvent"

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

	if data.TenantID == "" {
		return errors.New("tenant id is required")
	}

	if data.ContextID == "" {
		return errors.New("context id is required")
	}

	if err := model.ValidatePositiveAmountMinor(data.AmountMinor); err != nil {
		return fmt.Errorf("%s: amount minor: %w", op, err)
	}

	if !data.Currency.IsValid() {
		return errors.New("currency is invalid")
	}

	if !data.Channel.IsValid() {
		return errors.New("channel is invalid")
	}

	if data.PaymentPurpose != PaymentPurposeInstallment {
		return fmt.Errorf("%s: payment purpose must be %q", op, PaymentPurposeInstallment)
	}

	if data.SourceService != SourceService {
		return fmt.Errorf("%s: source service must be %q", op, SourceService)
	}

	if data.SourceType != SourceTypeBNPLPaymentAttempt {
		return fmt.Errorf("%s: source type must be %q", op, SourceTypeBNPLPaymentAttempt)
	}

	if data.SourceID == "" {
		return errors.New("source id is required")
	}

	if data.SourceID != data.BNPLPaymentAttemptID.String() {
		return errors.New("source id must match bnpl payment attempt id")
	}

	if data.SourceVersion <= 0 {
		return errors.New("source version is required")
	}

	if data.IdempotencyKey == "" {
		return errors.New("idempotency key is required")
	}

	if data.RequestFingerprint == "" {
		return errors.New("request fingerprint is required")
	}

	if len(data.Items) == 0 {
		return errors.New("items are required")
	}

	if total, ok := payableAmountFromEventItems(data.Items); !ok || total != data.AmountMinor {
		return errors.New("items total is inconsistent with amount minor")
	}

	return nil
}

func payableAmountFromEventItems(items []EventItem) (uint64, bool) {
	attemptItems := make([]model.PaymentAttemptItem, 0, len(items))
	for _, item := range items {
		attemptItems = append(attemptItems, model.PaymentAttemptItem{
			ItemType:    item.ItemType,
			SourceID:    item.SourceID,
			AmountMinor: item.AmountMinor,
			Effect:      item.Effect,
		})
	}

	return model.PayableAmountFromItems(attemptItems)
}
