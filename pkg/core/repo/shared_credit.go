package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// SharedCreditTxQueryer serializes credit assignment on a legal person and
// enrolls authenticated accounts into that person's existing capacity.
//
// All methods run inside the caller's transaction. They never perform external
// network I/O. Merchant and buyer read paths must not use these operations:
// enrollment is permission to spend, not a second credit limit.
type SharedCreditTxQueryer interface {
	// LockOwnerByCustomer locks credit_owners for the profile's legal person.
	LockOwnerByCustomer(ctx context.Context, customerID model.CustomerID) (model.CreditOwnerID, error)

	// LockByOwnerAndKind locks the single credit account for owner/currency/kind,
	// whether or not the calling customer is enrolled yet.
	LockByOwnerAndKind(
		ctx context.Context,
		ownerID model.CreditOwnerID,
		currency model.Currency,
		kind model.CreditKind,
	) (model.CreditAccount, error)

	// EnrollCustomer grants one account access to preexisting person capacity.
	// Identical enrollments are idempotent; an owner mismatch is a conflict.
	EnrollCustomer(
		ctx context.Context,
		customerID model.CustomerID,
		accountID model.CreditAccountID,
		ownerID model.CreditOwnerID,
		at time.Time,
	) (bool, error)
}

// SharedCredit is the persistence boundary for person-scoped credit locking
// and enrollment. Bindings follow the same Conn/Tx pattern as other repos;
// only transaction-scoped methods are exposed because every operation locks.
type SharedCredit interface {
	Tx(Tx) SharedCreditTxQueryer
}
