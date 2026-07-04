package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// InboxID uniquely identifies an inbox row.
type InboxID uuid.UUID

// ParseInboxID parses an InboxID from its canonical string form.
func ParseInboxID(value string) (InboxID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return InboxID{}, fmt.Errorf("parse inbox id: %w", err)
	}

	return InboxID(id), nil
}

func (id InboxID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id InboxID) String() string { return uuid.UUID(id).String() }

func (id InboxID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id InboxID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *InboxID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = InboxID(u)

	return nil
}

func (id *InboxID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = InboxID(u)

	return nil
}

func (id InboxID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// InboxStatus identifies durable inbound event processing state.
type InboxStatus string

// ReceivedInboxStatus and related values identify durable inbox processing state.
const (
	ReceivedInboxStatus               InboxStatus = "received"
	ProcessedInboxStatus              InboxStatus = "processed"
	ReconciliationRequiredInboxStatus InboxStatus = "reconciliation_required"
	FailedInboxStatus                 InboxStatus = "failed"
)

// InboxAcquireResult reports how an inbound event should be handled after acquisition.
type InboxAcquireResult string

// InboxAcquireNew and related values report how an inbound event should be handled after acquisition.
const (
	InboxAcquireNew                    InboxAcquireResult = "new"
	InboxAcquireAlreadyProcessed       InboxAcquireResult = "already_processed"
	InboxAcquireReconciliationRequired InboxAcquireResult = "reconciliation_required"
	InboxAcquirePermanentlyFailed      InboxAcquireResult = "permanently_failed"
	InboxAcquirePayloadMismatch        InboxAcquireResult = "payload_mismatch"
)

// InboxConsumerName identifies the integration consumer that owns an inbox row.
type InboxConsumerName string

// FinancialCorePaymentResultInboxConsumerName and related values name Financial Core inbox consumers.
const (
	FinancialCorePaymentResultInboxConsumerName                          InboxConsumerName = "financial_core_payment_result"
	FinancialCoreInstallmentPaymentSessionInboxConsumerName              InboxConsumerName = "financial_core_installment_payment_session"
	FinancialCoreInstallmentPaymentReconciliationResultInboxConsumerName InboxConsumerName = "financial_core_installment_payment_reconciliation_result"
)

// InboxSubjectType identifies the primary BNPL subject correlated to an inbox row.
type InboxSubjectType string

// ContractInboxSubjectType and related values identify BNPL subjects correlated to inbox rows.
const (
	ContractInboxSubjectType               InboxSubjectType = "contract"
	InstallmentInboxSubjectType            InboxSubjectType = "installment"
	PaymentAttemptInboxSubjectType         InboxSubjectType = "payment_attempt"
	FinancialPaymentIntentInboxSubjectType InboxSubjectType = "financial_payment_intent"
)

// InboxExternalReferenceType identifies an external reference stored on an inbox row.
type InboxExternalReferenceType string

// FinancialPaymentIntentInboxExternalReferenceType is the Financial Core payment-intent external reference type.
const (
	FinancialPaymentIntentInboxExternalReferenceType InboxExternalReferenceType = "financial_payment_intent"
)

// InboxEvent carries the durable event envelope used for inbox acquisition.
type InboxEvent struct {
	// EventID is the upstream event identity used for inbox idempotency.
	EventID string

	// EventType identifies the kind of inbound integration event.
	EventType string

	// EventVersion identifies the schema version of the inbound event.
	EventVersion string

	// SourceService identifies the upstream service that emitted the event.
	SourceService string

	// CorrelationID links the event to distributed tracing or business correlation.
	CorrelationID *string

	// CausationID optionally records the command or event that caused this event.
	CausationID *string

	// Payload contains the serialized inbound event body.
	Payload json.RawMessage

	// PayloadFingerprint is a stable hash used to detect payload identity conflicts.
	PayloadFingerprint string

	// ConsumerName identifies which BNPL inbox consumer owns this event.
	ConsumerName InboxConsumerName

	// SubjectType optionally identifies the local business subject type.
	SubjectType *string

	// SubjectID optionally identifies the local business subject identity.
	SubjectID *string

	// ExternalReferenceType optionally classifies the external reference value.
	ExternalReferenceType *string

	// ExternalReference optionally stores an upstream entity reference.
	ExternalReference *string
}

// IsValid reports whether the inbox status is known by the domain.
func (s InboxStatus) IsValid() bool {
	switch s {
	case ReceivedInboxStatus,
		ProcessedInboxStatus,
		ReconciliationRequiredInboxStatus,
		FailedInboxStatus:
		return true

	default:
		return false
	}
}

// Inbox stores a durable inbound integration event.
type Inbox struct {
	// ID uniquely identifies this inbox row.
	ID InboxID `db:"id" json:"id,omitempty"`

	// EventID is the upstream event identity used for inbox idempotency.
	EventID string `db:"event_id" json:"event_id,omitempty"`

	// EventType identifies the kind of inbound integration event.
	EventType string `db:"event_type" json:"event_type,omitempty"`

	// EventVersion identifies the schema version of the inbound event.
	EventVersion string `db:"event_version" json:"event_version,omitempty"`

	// SourceService identifies the upstream service that emitted the event.
	SourceService string `db:"source_service" json:"source_service,omitempty"`

	// CorrelationID links the event to distributed tracing or business correlation.
	CorrelationID *string `db:"correlation_id" json:"correlation_id,omitempty"`

	// CausationID optionally records the command or event that caused this event.
	CausationID *string `db:"causation_id" json:"causation_id,omitempty"`

	// Payload contains the serialized inbound event body.
	Payload json.RawMessage `db:"payload" json:"-"`

	// PayloadFingerprint is a stable hash used to detect payload identity conflicts.
	PayloadFingerprint string `db:"payload_fingerprint" json:"payload_fingerprint,omitempty"`

	// ConsumerName identifies which BNPL inbox consumer owns this event.
	ConsumerName string `db:"consumer_name" json:"consumer_name,omitempty"`

	// SubjectType optionally identifies the local business subject type.
	SubjectType *string `db:"subject_type" json:"subject_type,omitempty"`

	// SubjectID optionally identifies the local business subject identity.
	SubjectID *string `db:"subject_id" json:"subject_id,omitempty"`

	// ExternalReferenceType optionally classifies the external reference value.
	ExternalReferenceType *string `db:"external_reference_type" json:"external_reference_type,omitempty"`

	// ExternalReference optionally stores an upstream entity reference.
	ExternalReference *string `db:"external_reference" json:"external_reference,omitempty"`

	// Status is the current processing state of the inbox event.
	Status InboxStatus `db:"status" json:"status,omitempty"`

	// ReceivedAt is when the event was first accepted into the inbox.
	ReceivedAt time.Time `db:"received_at" json:"received_at,omitempty"`

	// ProcessedAt is when the event was successfully processed.
	ProcessedAt *time.Time `db:"processed_at" json:"processed_at,omitempty"`

	// FailedAt is when processing permanently failed for this event.
	FailedAt *time.Time `db:"failed_at" json:"failed_at,omitempty"`

	// FailureCode classifies why inbox processing failed.
	FailureCode *string `db:"failure_code" json:"failure_code,omitempty"`

	// FailureMessage stores a bounded human-readable processing failure detail.
	FailureMessage *string `db:"failure_message" json:"failure_message,omitempty"`

	// CreatedAt is when the inbox row was created.
	CreatedAt time.Time `db:"created_at" json:"created_at,omitempty"`

	// UpdatedAt is when the inbox row was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at,omitempty"`
}
