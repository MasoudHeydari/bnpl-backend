package installmentrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func (q connQueryer) ListCollectionsObligationsForCreditOwner(ctx context.Context, customer model.CustomerID) ([]repo.CollectionsObligation, error) {
	return ListCollectionsObligationsForCreditOwner(ctx, q.Conn, customer)
}

func (q txQueryer) ListCollectionsObligationsForCreditOwner(ctx context.Context, customer model.CustomerID) ([]repo.CollectionsObligation, error) {
	return ListCollectionsObligationsForCreditOwner(ctx, q.Tx, customer)
}

func (q connQueryer) HasDueUnpaidForCreditOwner(ctx context.Context, customer model.CustomerID, asOf time.Time) (bool, error) {
	return hasDueUnpaidForCreditOwner(ctx, q.Conn, customer, asOf)
}

func (q txQueryer) HasDueUnpaidForCreditOwner(ctx context.Context, customer model.CustomerID, asOf time.Time) (bool, error) {
	return hasDueUnpaidForCreditOwner(ctx, q.Tx, customer, asOf)
}

func hasDueUnpaidForCreditOwner[Q postgres.Queryer](ctx context.Context, q Q, customer model.CustomerID, asOf time.Time) (bool, error) {
	const query = `SELECT EXISTS (
        SELECT 1 FROM installments i JOIN contracts c ON c.id=i.contract_id
        WHERE c.credit_owner_id=(SELECT credit_owner_id FROM customer_profiles WHERE id=$1)
          AND c.status IN ('active','canceling')
          AND i.status IN ('scheduled','due','overdue')
          AND (i.due_at<=$2 OR i.status IN ('due','overdue'))
    ) AS due`
	result, err := postgres.QueryOne[struct {
		Due bool `db:"due"`
	}](
		ctx,
		q,
		"person has due unpaid installments",
		query,
		customer,
		asOf,
	)
	return result.Due, err
}
