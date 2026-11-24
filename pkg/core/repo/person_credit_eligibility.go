package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Person-level eligibility readers inspect all accounts belonging to a legal
// person. Account-owned repayment and history remain separately scoped.

// PersonCreditRestrictionReader loads effective restrictions for a customer's legal person.
type PersonCreditRestrictionReader interface {
	ListActiveForCreditOwner(ctx context.Context, requestingCustomer model.CustomerID) ([]model.CustomerRestriction, error)
}

// PersonCreditEpisodeReader checks whether that person has an active default episode.
type PersonCreditEpisodeReader interface {
	HasActiveForCreditOwner(ctx context.Context, requestingCustomer model.CustomerID) (bool, error)
}

// PersonCreditObligationReader loads obligations for person-level eligibility.
type PersonCreditObligationReader interface {
	ListCollectionsObligationsForCreditOwner(ctx context.Context, requestingCustomer model.CustomerID) ([]CollectionsObligation, error)
}

// PersonCreditDueReader checks unpaid amounts due across that person's accounts.
type PersonCreditDueReader interface {
	HasDueUnpaidForCreditOwner(ctx context.Context, requestingCustomer model.CustomerID, asOf time.Time) (bool, error)
}
