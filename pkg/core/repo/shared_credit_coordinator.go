package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// SharedCreditCoordinator serializes new credit assignments on the legal
// person and enrolls authenticated accounts in that person's existing credit.
// Every operation uses the caller's transaction; no external calls are made.
type SharedCreditCoordinator interface {
	LockOwnerByCustomer(ctx context.Context, tx Tx, customerID model.CustomerID) (model.CreditOwnerID, error)
	LockByOwnerAndKind(ctx context.Context, tx Tx, ownerID model.CreditOwnerID, currency model.Currency, kind model.CreditKind) (model.CreditAccount, error)
	EnrollCustomer(ctx context.Context, tx Tx, customerID model.CustomerID, accountID model.CreditAccountID, ownerID model.CreditOwnerID, at time.Time) (bool, error)
}
