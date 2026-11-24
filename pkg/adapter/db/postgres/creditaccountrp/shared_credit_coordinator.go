package creditaccountrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.SharedCreditCoordinator = (*SharedCoordinator)(nil)

// SharedCoordinator implements mandatory person-scoped credit locking.
type SharedCoordinator struct{}

// NewSharedCoordinator constructs a mandatory credit-owner locking collaborator.
func NewSharedCoordinator() *SharedCoordinator { return &SharedCoordinator{} }

// LockOwnerByCustomer locks the person owning an authenticated account.
func (*SharedCoordinator) LockOwnerByCustomer(
	ctx context.Context,
	tx repo.Tx,
	customerID model.CustomerID,
) (model.CreditOwnerID, error) {
	const op = "creditaccountrp.SharedCoordinator.LockOwnerByCustomer"
	type result struct {
		ID model.CreditOwnerID `db:"id"`
	}

	const query = `SELECT co.id FROM credit_owners co
        JOIN customer_profiles cp ON cp.credit_owner_id = co.id
        WHERE cp.id=$1 FOR UPDATE OF co`
	row, err := postgres.QueryOne[result](ctx, postgres.MustTx(tx), "lock credit owner", query, customerID)
	if err != nil {
		return model.CreditOwnerID{}, fmt.Errorf("%s: lock legal person: %w", op, err)
	}

	return row.ID, nil
}

// LockByOwnerAndKind locks the one shared credit account of the requested type.
func (*SharedCoordinator) LockByOwnerAndKind(
	ctx context.Context,
	tx repo.Tx,
	owner model.CreditOwnerID,
	currency model.Currency,
	kind model.CreditKind,
) (model.CreditAccount, error) {
	q := txQueryer{Tx: postgres.MustTx(tx)}
	return q.LockByCreditOwnerAndKind(ctx, owner, currency, kind)
}

// EnrollCustomer grants an account access to a preexisting person-owned credit.
func (*SharedCoordinator) EnrollCustomer(
	ctx context.Context,
	tx repo.Tx,
	customerID model.CustomerID,
	accountID model.CreditAccountID,
	ownerID model.CreditOwnerID,
	at time.Time,
) (bool, error) {
	q := txQueryer{Tx: postgres.MustTx(tx)}
	return q.EnrollCustomer(ctx, customerID, accountID, ownerID, at)
}
