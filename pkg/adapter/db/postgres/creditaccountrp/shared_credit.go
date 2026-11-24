package creditaccountrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.SharedCredit = (*SharedCredit)(nil)
var _ repo.SharedCreditTxQueryer = sharedCreditTxQueryer{}

// SharedCredit implements the person-scoped credit locking and enrollment port.
type SharedCredit struct{}

// NewSharedCredit constructs the SharedCredit repository adapter.
func NewSharedCredit() *SharedCredit { return &SharedCredit{} }

// Tx binds shared-credit operations to the caller's transaction.
func (*SharedCredit) Tx(tx repo.Tx) repo.SharedCreditTxQueryer {
	return sharedCreditTxQueryer{Tx: postgres.MustTx(tx)}
}

type sharedCreditTxQueryer struct{ *postgres.Tx }

// LockOwnerByCustomer locks the legal person that owns the authenticated account.
func (q sharedCreditTxQueryer) LockOwnerByCustomer(
	ctx context.Context,
	customerID model.CustomerID,
) (model.CreditOwnerID, error) {
	const op = "creditaccountrp.SharedCredit.LockOwnerByCustomer"
	type result struct {
		ID model.CreditOwnerID `db:"id"`
	}

	const query = `SELECT co.id FROM credit_owners co
        JOIN customer_profiles cp ON cp.credit_owner_id = co.id
        WHERE cp.id=$1 FOR UPDATE OF co`
	row, err := postgres.QueryOne[result](
		ctx,
		q.Tx,
		"lock credit owner",
		query,
		customerID,
	)
	if err != nil {
		return model.CreditOwnerID{}, fmt.Errorf("%s: lock legal person: %w", op, err)
	}

	return row.ID, nil
}

// LockByOwnerAndKind locks the one shared credit account of the requested type.
func (q sharedCreditTxQueryer) LockByOwnerAndKind(
	ctx context.Context,
	owner model.CreditOwnerID,
	currency model.Currency,
	kind model.CreditKind,
) (model.CreditAccount, error) {
	return lockByCreditOwnerAndKind(ctx, q.Tx, owner, currency, kind)
}

// EnrollCustomer grants an account access to a preexisting person-owned credit.
func (q sharedCreditTxQueryer) EnrollCustomer(
	ctx context.Context,
	customerID model.CustomerID,
	accountID model.CreditAccountID,
	ownerID model.CreditOwnerID,
	at time.Time,
) (bool, error) {
	return enrollCustomer(ctx, q.Tx, customerID, accountID, ownerID, at)
}

// lockByCreditOwnerAndKind finds the one governed credit capacity for a legal
// person, whether or not the invoking customer has enrolled yet. This helper
// MUST NOT be called by merchant/buyer read endpoints.
func lockByCreditOwnerAndKind[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	owner model.CreditOwnerID,
	currency model.Currency,
	kind model.CreditKind,
) (model.CreditAccount, error) {
	if owner.IsZero() || !currency.IsValid() || !kind.IsValid() {
		return model.CreditAccount{}, cerr.InvalidArgument(fmt.Errorf("invalid shared credit account lookup"))
	}

	const query = `SELECT ` + creditAccountColumns + `
FROM credit_accounts
WHERE credit_owner_id = $1 AND currency = $2 AND credit_kind = $3
FOR UPDATE`
	return postgres.QueryOne[model.CreditAccount](
		ctx,
		q,
		"lock shared credit account",
		query,
		owner,
		currency,
		kind,
	)
}

// enrollCustomer only creates permission to use existing capacity. PostgreSQL
// composite FKs ensure that this customer and credit account have one owner.
func enrollCustomer[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	accountID model.CreditAccountID,
	ownerID model.CreditOwnerID,
	now time.Time,
) (bool, error) {
	if customerID.IsZero() || accountID.IsZero() || ownerID.IsZero() {
		return false, cerr.InvalidArgument(fmt.Errorf("credit enrollment requires customer, account and owner"))
	}

	if err := postgres.RequiredTime("enrolled_at", now); err != nil {
		return false, err
	}

	const query = `INSERT INTO customer_credit_enrollments (customer_id, credit_account_id, credit_owner_id, enrolled_at)
VALUES ($1,$2,$3,$4)
ON CONFLICT (customer_id,credit_account_id) DO NOTHING`
	n, err := q.Exec(
		ctx,
		query,
		customerID,
		accountID,
		ownerID,
		now,
	)
	if err != nil {
		return false, fmt.Errorf("enroll customer credit: %w", postgres.MapError(err))
	}

	// If already present, still ensure the owner is correct by loading the
	// enrollment whose credit_owner_id is immutable.
	if n == 0 {
		const ownerLookup = `SELECT EXISTS (
            SELECT 1 FROM customer_credit_enrollments
            WHERE customer_id=$1 AND credit_account_id=$2 AND credit_owner_id=$3
        ) AS matches`
		type result struct {
			Matches bool `db:"matches"`
		}

		row, err := postgres.QueryOne[result](
			ctx,
			q,
			"check existing enrollment",
			ownerLookup,
			customerID,
			accountID,
			ownerID,
		)
		if err != nil {
			return false, err
		}

		if !row.Matches {
			return false, cerr.Conflict(fmt.Errorf("existing credit enrollment owner mismatch"))
		}
	}

	return n == 1, nil
}
