package repo

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// PendingPersonContractQueryer is a transaction-scoped diagnostic lookup used
// only after the database rejects concurrent pre-activation contract creation.
// It does not authorize access to the returned customer's contract.
type PendingPersonContractQueryer interface {
	GetPaymentPendingByCreditOwnerID(ctx context.Context, ownerID model.CreditOwnerID) (model.Contract, error)
}
