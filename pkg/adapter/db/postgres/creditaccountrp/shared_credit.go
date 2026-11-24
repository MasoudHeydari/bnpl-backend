package creditaccountrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// LockByCreditOwnerAndKind finds the one governed credit capacity for a legal
// person, whether or not the invoking customer has enrolled yet. This method
// MUST NOT be called by merchant/buyer read endpoints.
func (q txQueryer) LockByCreditOwnerAndKind(ctx context.Context, owner model.CreditOwnerID, currency model.Currency, kind model.CreditKind) (model.CreditAccount, error) {
	if owner.IsZero() || !currency.IsValid() || !kind.IsValid() {
		return model.CreditAccount{}, cerr.InvalidArgument(fmt.Errorf("invalid shared credit account lookup"))
	}
	const query = `SELECT ` + creditAccountColumns + `
FROM credit_accounts
WHERE credit_owner_id = $1 AND currency = $2 AND credit_kind = $3
FOR UPDATE`
	return postgres.QueryOne[model.CreditAccount](ctx, q.Tx, "lock shared credit account", query, owner, currency, kind)
}

// EnrollCustomer only creates permission to use existing capacity. PostgreSQL
// composite FKs ensure that this customer and credit account have one owner.
func (q txQueryer) EnrollCustomer(ctx context.Context, customerID model.CustomerID, accountID model.CreditAccountID, ownerID model.CreditOwnerID, now time.Time) (bool, error) {
	if customerID.IsZero() || accountID.IsZero() || ownerID.IsZero() {
		return false, cerr.InvalidArgument(fmt.Errorf("credit enrollment requires customer, account and owner"))
	}
	if err := postgres.RequiredTime("enrolled_at", now); err != nil {
		return false, err
	}
	const query = `INSERT INTO customer_credit_enrollments (customer_id, credit_account_id, credit_owner_id, enrolled_at)
VALUES ($1,$2,$3,$4)
ON CONFLICT (customer_id,credit_account_id) DO NOTHING`
	n, err := q.Exec(ctx, query, customerID, accountID, ownerID, now)
	if err != nil {
		return false, fmt.Errorf("enroll customer credit: %w", postgres.MapError(err))
	}
	// If already present, still ensure the owner is correct by loading the
	// source customer, whose credit_owner_id is immutable.
	if n == 0 {
		const ownerLookup = `SELECT EXISTS (
            SELECT 1 FROM customer_credit_enrollments
            WHERE customer_id=$1 AND credit_account_id=$2 AND credit_owner_id=$3
        ) AS matches`
		type result struct {
			Matches bool `db:"matches"`
		}
		row, err := postgres.QueryOne[result](ctx, q.Tx, "check existing enrollment", ownerLookup, customerID, accountID, ownerID)
		if err != nil {
			return false, err
		}
		if !row.Matches {
			return false, cerr.Conflict(fmt.Errorf("existing credit enrollment owner mismatch"))
		}
	}
	return n == 1, nil
}
