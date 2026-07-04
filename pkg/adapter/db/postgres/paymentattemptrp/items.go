package paymentattemptrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const paymentAttemptItemColumns = `
	id,
	payment_attempt_id,
	item_type,
	source_id,
	amount_minor,
	effect,
	created_at
`

func listItemsByPaymentAttemptID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	paymentAttemptID model.PaymentAttemptID,
) ([]model.PaymentAttemptItem, error) {
	const query = `SELECT` + paymentAttemptItemColumns + `
FROM payment_attempt_items
WHERE payment_attempt_id = $1
ORDER BY created_at ASC, id ASC
`
	const op = "paymentattemptrp.listItemsByPaymentAttemptID"

	rows, err := q.Query(ctx, query, paymentAttemptID)
	if err != nil {
		return nil, fmt.Errorf("%s: list payment attempt items by payment attempt id: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	return postgres.ValuesInto[model.PaymentAttemptItem](postgres.MustRows(rows))
}

func createItems[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	paymentAttemptID model.PaymentAttemptID,
	items []model.PaymentAttemptItem,
	createdAt time.Time,
) error {
	const op = "paymentattemptrp.createItems"

	if len(items) == 0 {
		return nil
	}

	if err := postgres.RequiredTimes(map[string]time.Time{
		"created_at": createdAt,
	}); err != nil {
		return err
	}

	const query = `
INSERT INTO payment_attempt_items (
	payment_attempt_id,
	item_type,
	source_id,
	amount_minor,
	effect,
	created_at
)
VALUES ($1, $2, $3, $4, $5, $6)
`

	for _, item := range items {
		if _, err := q.Exec(
			ctx,
			query,
			paymentAttemptID,
			item.ItemType,
			item.SourceID,
			item.AmountMinor,
			item.Effect,
			createdAt,
		); err != nil {
			return fmt.Errorf("%s: create payment attempt item: %w", op, postgres.MapError(err))
		}
	}

	return nil
}

func attachItems(attempt model.PaymentAttempt, items []model.PaymentAttemptItem) model.PaymentAttempt {
	for i := range items {
		items[i].PaymentAttemptID = attempt.ID
	}

	attempt.Items = items

	return attempt
}
