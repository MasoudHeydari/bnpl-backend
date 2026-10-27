package repaymentreminderrp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const reminderColumns = `
	id,
	customer_id,
	external_customer_id,
	business_date,
	reminder_type,
	policy_version,
	template_id,
	short_link_id,
	payment_url,
	installment_ids,
	notification_idempotency_key,
	created_at
`


type reminderRow struct {
	ID                         model.RepaymentReminderID `db:"id"`
	CustomerID                 model.CustomerID          `db:"customer_id"`
	ExternalCustomerID         model.ExternalCustomerID  `db:"external_customer_id"`
	BusinessDate               time.Time                 `db:"business_date"`
	ReminderType               model.ReminderType        `db:"reminder_type"`
	PolicyVersion              string                    `db:"policy_version"`
	TemplateID                 string                    `db:"template_id"`
	ShortLinkID                model.ShortLinkID         `db:"short_link_id"`
	PaymentURL                 string                    `db:"payment_url"`
	InstallmentIDs             []uuid.UUID               `db:"installment_ids"`
	NotificationIdempotencyKey string                    `db:"notification_idempotency_key"`
	CreatedAt                  time.Time                 `db:"created_at"`
}

func (r reminderRow) toModel() model.RepaymentReminder {
	ids := make([]model.InstallmentID, 0, len(r.InstallmentIDs))
	for _, id := range r.InstallmentIDs {
		ids = append(ids, model.InstallmentID(id))
	}

	return model.RepaymentReminder{
		ID:                         r.ID,
		CustomerID:                 r.CustomerID,
		ExternalCustomerID:         r.ExternalCustomerID,
		BusinessDate:               r.BusinessDate,
		ReminderType:               r.ReminderType,
		PolicyVersion:              r.PolicyVersion,
		TemplateID:                 r.TemplateID,
		ShortLinkID:                r.ShortLinkID,
		PaymentURL:                 r.PaymentURL,
		InstallmentIDs:             ids,
		NotificationIdempotencyKey: r.NotificationIdempotencyKey,
		CreatedAt:                  r.CreatedAt,
	}
}

func installmentUUIDs(ids []model.InstallmentID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.UUID())
	}

	return out
}

func dateOnlyUTC(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// GetByCustomerBusinessDate returns the reminder for a customer on a business date.
func GetByCustomerBusinessDate[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	businessDate time.Time,
) (model.RepaymentReminder, error) {
	const op = "repaymentreminderrp.GetByCustomerBusinessDate"

	if customerID.IsZero() {
		return model.RepaymentReminder{}, cerr.InternalError(
			fmt.Errorf("%s: customer id is required", op),
		)
	}

	const query = `SELECT` + reminderColumns + `
FROM repayment_reminders
WHERE customer_id = $1
  AND business_date = $2
`

	row, err := postgres.QueryOne[reminderRow](
		ctx,
		q,
		"get repayment reminder by customer business date",
		query,
		customerID,
		dateOnlyUTC(businessDate),
	)
	if err != nil {
		return model.RepaymentReminder{}, err
	}

	return row.toModel(), nil
}

// GetByID returns a repayment reminder by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.RepaymentReminderID,
) (model.RepaymentReminder, error) {
	const op = "repaymentreminderrp.GetByID"

	if id.IsZero() {
		return model.RepaymentReminder{}, cerr.InternalError(
			fmt.Errorf("%s: id is required", op),
		)
	}

	const query = `SELECT` + reminderColumns + `
FROM repayment_reminders
WHERE id = $1
`

	row, err := postgres.QueryOne[reminderRow](
		ctx,
		q,
		"get repayment reminder by id",
		query,
		id,
	)
	if err != nil {
		return model.RepaymentReminder{}, err
	}

	return row.toModel(), nil
}

// LockByCustomerBusinessDate locks the reminder row for a customer/day when present.
func LockByCustomerBusinessDate[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	businessDate time.Time,
) (model.RepaymentReminder, error) {
	const op = "repaymentreminderrp.LockByCustomerBusinessDate"

	if customerID.IsZero() {
		return model.RepaymentReminder{}, cerr.InternalError(
			fmt.Errorf("%s: customer id is required", op),
		)
	}

	const query = `SELECT` + reminderColumns + `
FROM repayment_reminders
WHERE customer_id = $1
  AND business_date = $2
FOR UPDATE
`

	row, err := postgres.QueryOne[reminderRow](
		ctx,
		q,
		"lock repayment reminder by customer business date",
		query,
		customerID,
		dateOnlyUTC(businessDate),
	)
	if err != nil {
		return model.RepaymentReminder{}, err
	}

	return row.toModel(), nil
}

// Create stores a reminder request.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	message repo.CreateRepaymentReminder,
) (model.RepaymentReminder, error) {
	const op = "repaymentreminderrp.Create"

	if message.ID.IsZero() {
		return model.RepaymentReminder{}, cerr.InternalError(
			fmt.Errorf("%s: id is required", op),
		)
	}

	if message.CustomerID.IsZero() {
		return model.RepaymentReminder{}, cerr.InternalError(
			fmt.Errorf("%s: customer id is required", op),
		)
	}

	if message.ExternalCustomerID.IsZero() {
		return model.RepaymentReminder{}, cerr.InternalError(
			fmt.Errorf("%s: external customer id is required", op),
		)
	}

	if !message.ReminderType.IsValid() {
		return model.RepaymentReminder{}, cerr.InternalError(
			fmt.Errorf("%s: reminder type is invalid", op),
		)
	}

	if message.ShortLinkID.IsZero() {
		return model.RepaymentReminder{}, cerr.InternalError(
			fmt.Errorf("%s: short link id is required", op),
		)
	}

	if len(message.InstallmentIDs) == 0 {
		return model.RepaymentReminder{}, cerr.InternalError(
			fmt.Errorf("%s: installment ids are required", op),
		)
	}

	// ON CONFLICT DO NOTHING keeps the surrounding transaction usable when a
	// concurrent worker already persisted the customer/day row. Empty RETURNING
	// maps to ResourceNotFound which we translate to Conflict.
	const query = `
INSERT INTO repayment_reminders (
	id,
	customer_id,
	external_customer_id,
	business_date,
	reminder_type,
	policy_version,
	template_id,
	short_link_id,
	payment_url,
	installment_ids,
	notification_idempotency_key,
	created_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
ON CONFLICT DO NOTHING
RETURNING` + reminderColumns

	row, err := postgres.QueryOne[reminderRow](
		ctx,
		q,
		"create repayment reminder",
		query,
		message.ID,
		message.CustomerID,
		message.ExternalCustomerID,
		dateOnlyUTC(message.BusinessDate),
		message.ReminderType,
		message.PolicyVersion,
		message.TemplateID,
		message.ShortLinkID,
		message.PaymentURL,
		installmentUUIDs(message.InstallmentIDs),
		message.NotificationIdempotencyKey,
		message.CreatedAt.UTC(),
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.RepaymentReminder{}, cerr.Conflict(
				fmt.Errorf("%s: repayment reminder already exists", op),
			)
		}

		return model.RepaymentReminder{}, err
	}

	return row.toModel(), nil
}
