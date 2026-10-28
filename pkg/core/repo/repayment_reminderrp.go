package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreateRepaymentReminder carries fields required to persist a reminder request.
type CreateRepaymentReminder struct {
	// ID uniquely identifies the reminder request.
	ID model.RepaymentReminderID

	// CustomerID is the BNPL-local customer_profiles.id.
	CustomerID model.CustomerID

	// ExternalCustomerID is the Identity / Customer Reference identifier.
	ExternalCustomerID model.ExternalCustomerID

	// BusinessDate is the calendar date in the business timezone (date-only).
	BusinessDate time.Time

	// ReminderType is the selected semantic reminder class.
	ReminderType model.ReminderType

	// PolicyVersion identifies the policy that produced this row.
	PolicyVersion string

	// TemplateID is the Notification Service template identifier.
	TemplateID string

	// ShortLinkID references the unique Short Link created for this Reminder.
	ShortLinkID model.ShortLinkID

	// PaymentURL is the compact public short-link URL for the SMS body.
	PaymentURL string

	// InstallmentIDs lists the installment(s) that justified the selected type.
	InstallmentIDs []model.InstallmentID

	// NotificationIdempotencyKey is the stable Notification Service idempotency key.
	NotificationIdempotencyKey string

	// CreatedAt is when the reminder request should record as created.
	CreatedAt time.Time
}

// RepaymentReminderQueryer defines read-only repayment reminder operations.
type RepaymentReminderQueryer interface {
	// GetByNotificationIdempotencyKey finds an episode-scoped durable request.
	GetByNotificationIdempotencyKey(context.Context, string) (model.RepaymentReminder, error)
	// GetByCustomerBusinessDate returns the reminder for a customer on a business date.
	GetByCustomerBusinessDate(
		ctx context.Context,
		customerID model.CustomerID,
		businessDate time.Time,
	) (model.RepaymentReminder, error)

	// GetByID returns a repayment reminder by id.
	GetByID(ctx context.Context, id model.RepaymentReminderID) (model.RepaymentReminder, error)
}

// RepaymentReminderConnQueryer exposes reminder operations on a connection.
type RepaymentReminderConnQueryer interface {
	// RepaymentReminderQueryer provides shared reminder query operations.
	RepaymentReminderQueryer
}

// RepaymentReminderTxQueryer exposes transaction-scoped reminder operations.
type RepaymentReminderTxQueryer interface {
	// RepaymentReminderQueryer provides shared reminder query operations.
	RepaymentReminderQueryer

	// Create stores a reminder request.
	//
	// Unique (customer_id, business_date) collisions must be detected without
	// aborting the surrounding PostgreSQL transaction (for example INSERT ...
	// ON CONFLICT DO NOTHING RETURNING mapped to Conflict). Callers treat
	// Conflict as a durable daily-limit hit and roll back the whole attempt.
	Create(ctx context.Context, message CreateRepaymentReminder) (model.RepaymentReminder, error)

	// LockByCustomerBusinessDate locks the reminder row for a customer/day when
	// present. Returns ResourceNotFound when no row exists yet.
	LockByCustomerBusinessDate(
		ctx context.Context,
		customerID model.CustomerID,
		businessDate time.Time,
	) (model.RepaymentReminder, error)
}

// RepaymentReminder defines repository bindings for repayment reminder requests.
type RepaymentReminder interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(conn Conn) RepaymentReminderConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(tx Tx) RepaymentReminderTxQueryer
}
