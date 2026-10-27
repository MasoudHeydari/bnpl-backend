package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreateCustomerRestriction carries fields required to apply a restriction.
type CreateCustomerRestriction struct {
	ID                model.CustomerRestrictionID
	CustomerID        model.CustomerID
	RestrictionType   model.CustomerRestrictionType
	Reason            model.CustomerRestrictionReason
	DefaultEpisodeID  *model.DefaultEpisodeID
	OpeningDecisionID model.DecisionID
	AppliedAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// CustomerRestrictionQueryer defines read-only customer restriction operations.
type CustomerRestrictionQueryer interface {
	GetActiveByCustomerType(
		ctx context.Context,
		customerID model.CustomerID,
		restrictionType model.CustomerRestrictionType,
	) (model.CustomerRestriction, error)

	ListActiveByCustomerID(
		ctx context.Context,
		customerID model.CustomerID,
	) ([]model.CustomerRestriction, error)
}

// CustomerRestrictionConnQueryer exposes restriction operations on a connection.
type CustomerRestrictionConnQueryer interface {
	CustomerRestrictionQueryer
}

// CustomerRestrictionTxQueryer exposes transaction-scoped restriction operations.
type CustomerRestrictionTxQueryer interface {
	CustomerRestrictionQueryer

	LockActiveByCustomerType(
		ctx context.Context,
		customerID model.CustomerID,
		restrictionType model.CustomerRestrictionType,
	) (model.CustomerRestriction, error)

	Create(ctx context.Context, message CreateCustomerRestriction) (model.CustomerRestriction, error)

	Release(
		ctx context.Context,
		id model.CustomerRestrictionID,
		releasingDecisionID model.DecisionID,
		releasedAt time.Time,
		updatedAt time.Time,
	) (model.CustomerRestriction, error)
}

// CustomerRestriction defines repository bindings for customer restrictions.
type CustomerRestriction interface {
	Conn(conn Conn) CustomerRestrictionConnQueryer
	Tx(tx Tx) CustomerRestrictionTxQueryer
}
