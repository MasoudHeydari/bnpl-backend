package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreditAllocationTxQueryer persists allocation configuration and request
// state. LockConfiguration must precede owner/request locks; all configuration
// writers acquire its exclusive form. No method performs external calls.
type CreditAllocationTxQueryer interface {
	LockConfiguration(context.Context, bool) error
	GetGroup(context.Context, string) (model.CreditGroup, error)
	ListGroups(context.Context) ([]model.CreditGroup, error)
	PutGroup(context.Context, model.CreditGroup) (model.CreditGroup, error)
	GetEligibilityByNationalCode(context.Context, string) (model.CreditEligibilityEntry, error)
	GetEligibilityByMobile(context.Context, string) (model.CreditEligibilityEntry, error)
	GetEligibility(context.Context, uuid.UUID) (model.CreditEligibilityEntry, error)
	InsertEligibility(context.Context, model.CreditEligibilityEntry) (model.CreditEligibilityEntry, error)
	InsertEligibilityMobile(context.Context, model.CreditEligibilityMobile) error
	ListEligibility(context.Context, *string, *uuid.UUID, int) ([]model.CreditEligibilityEntry, error)
	ListEligibilityMobiles(context.Context, uuid.UUID) ([]model.CreditEligibilityMobile, error)
	BindEligibility(context.Context, uuid.UUID, model.CreditOwnerID) error
	GetMembership(context.Context, uuid.UUID) (model.CreditGroupMembership, error)
	PutMembership(context.Context, model.CreditGroupMembership) (model.CreditGroupMembership, error)
	GetRule(context.Context, model.CreditAllocationScope, *string, *uuid.UUID, model.CreditKind, model.Currency) (model.CreditAllocationRule, error)
	PutRule(context.Context, model.CreditAllocationRule) (model.CreditAllocationRule, error)
	ListRules(context.Context) ([]model.CreditAllocationRule, error)
	GetOpenRequest(context.Context, model.CreditOwnerID, model.CreditKind, model.Currency) (model.CreditRequest, error)
	GetLatestRequest(context.Context, model.CustomerID, model.CreditKind, model.Currency) (model.CreditRequest, error)
	GetRequest(context.Context, uuid.UUID, bool) (model.CreditRequest, error)
	InsertRequest(context.Context, model.CreditRequest) (model.CreditRequest, error)
	QueueIdentityRecheck(context.Context, uuid.UUID, time.Time) error
	ClaimRequest(context.Context, uuid.UUID, time.Time, time.Time) (model.CreditRequest, error)
	QueuePublicRequests(context.Context, time.Time, int) error
	ListDueRequests(context.Context, time.Time, int) ([]model.CreditRequest, error)
	// FinishRequest compares the lease token and releases it atomically. A nil
	// next attempt defers a pending request until eligibility changes or public assessment availability.
	FinishRequest(context.Context, model.CreditRequest, model.CreditRequestStatus, string, *model.DecisionID, *uuid.UUID, *time.Time, time.Time) (model.CreditRequest, error)
	QueueEligibility(context.Context, *uuid.UUID, *string, *model.CreditKind, *model.Currency, time.Time) error

	ListPendingRequests(context.Context, model.CreditKind, time.Time, int) ([]model.CreditRequest, error)
}

// CreditAllocation is the transaction-scoped persistence port.
type CreditAllocation interface {
	Tx(Tx) CreditAllocationTxQueryer
}
