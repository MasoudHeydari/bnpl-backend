package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RepaymentReminderID uniquely identifies a persisted repayment reminder request.
type RepaymentReminderID uuid.UUID

// ParseRepaymentReminderID parses a RepaymentReminderID from its canonical string form.
func ParseRepaymentReminderID(value string) (RepaymentReminderID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return RepaymentReminderID{}, fmt.Errorf("parse repayment reminder id: %w", err)
	}

	return RepaymentReminderID(id), nil
}

func (id RepaymentReminderID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id RepaymentReminderID) String() string { return uuid.UUID(id).String() }

func (id RepaymentReminderID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id RepaymentReminderID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

func (id *RepaymentReminderID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = RepaymentReminderID(u)

	return nil
}

func (id *RepaymentReminderID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = RepaymentReminderID(u)

	return nil
}

func (id RepaymentReminderID) Value() (driver.Value, error) {
	return uuid.UUID(id).Value()
}

// ReminderType identifies the semantic repayment reminder template class.
type ReminderType string

// Known repayment reminder types.
const (
	PreDueReminderType        ReminderType = "pre_due"
	DueTodayReminderType      ReminderType = "due_today"
	GracePeriodReminderType   ReminderType = "grace_period"
	GraceFinalDayReminderType ReminderType = "grace_final_day"
)

// IsValid reports whether the reminder type is a known enumeration value.
func (t ReminderType) IsValid() bool {
	switch t {
	case PreDueReminderType,
		DueTodayReminderType,
		GracePeriodReminderType,
		GraceFinalDayReminderType:
		return true

	default:
		return false
	}
}

// Priority returns a higher number for higher-urgency reminder types.
func (t ReminderType) Priority() int {
	switch t {
	case GraceFinalDayReminderType:
		return 4

	case GracePeriodReminderType:
		return 3

	case DueTodayReminderType:
		return 2

	case PreDueReminderType:
		return 1

	default:
		return 0
	}
}

// RepaymentReminder is a durable customer-day reminder request audit record.
//
// Uniqueness on (CustomerID, BusinessDate) enforces at most one reminder SMS
// request per customer per business day. Delivery is asynchronous via the
// transactional outbox; this row is the business idempotency evidence.
type RepaymentReminder struct {
	// ID uniquely identifies the reminder request.
	ID RepaymentReminderID `db:"id" json:"id"`

	// CustomerID is the BNPL-local customer_profiles.id.
	CustomerID CustomerID `db:"customer_id" json:"customer_id"`

	// ExternalCustomerID is the Identity / Customer Reference identifier used
	// by Notification Service to resolve the recipient phone number.
	ExternalCustomerID ExternalCustomerID `db:"external_customer_id" json:"external_customer_id"`

	// BusinessDate is the calendar date in the business timezone (date-only).
	BusinessDate time.Time `db:"business_date" json:"business_date"`

	// ReminderType is the selected semantic reminder class.
	ReminderType ReminderType `db:"reminder_type" json:"reminder_type"`

	// PolicyVersion identifies the repayment-reminder policy that produced this row.
	PolicyVersion string `db:"policy_version" json:"policy_version"`

	// TemplateID is the Notification Service template identifier.
	TemplateID string `db:"template_id" json:"template_id"`

	// PaymentURL is the compact public short-link URL included in the SMS
	// (scheme omitted for display, e.g. dpay.ir/pay).
	PaymentURL string `db:"payment_url" json:"payment_url"`

	// InstallmentIDs lists the installment(s) that justified the selected type.
	InstallmentIDs []InstallmentID `db:"installment_ids" json:"installment_ids"`

	// NotificationIdempotencyKey is the stable key for Notification Service
	// idempotent processing (typically repayment-reminder:<reminder-id>).
	NotificationIdempotencyKey string `db:"notification_idempotency_key" json:"notification_idempotency_key"`

	// CreatedAt is when the reminder request was persisted.
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
