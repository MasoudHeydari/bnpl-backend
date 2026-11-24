package repo

import (
	"context"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"time"
)

// These person-wide read ports are strictly for new-credit eligibility.
// Account-owned installment, collections, repayment and history read ports
// intentionally remain scoped to one CustomerProfile.
type PersonCreditRestrictionReader interface {
	ListActiveForCreditOwner(ctx context.Context, requestingCustomer model.CustomerID) ([]model.CustomerRestriction, error)
}
type PersonCreditEpisodeReader interface {
	HasActiveForCreditOwner(ctx context.Context, requestingCustomer model.CustomerID) (bool, error)
}
type PersonCreditObligationReader interface {
	ListCollectionsObligationsForCreditOwner(ctx context.Context, requestingCustomer model.CustomerID) ([]CollectionsObligation, error)
}
type PersonCreditDueReader interface {
	HasDueUnpaidForCreditOwner(ctx context.Context, requestingCustomer model.CustomerID, asOf time.Time) (bool, error)
}
