package customerrestrictionrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const restrictionColumns = `
	id,
	customer_id,
	restriction_type,
	reason,
	status,
	default_episode_id,
	opening_decision_id,
	releasing_decision_id,
	applied_at,
	released_at,
	created_at,
	updated_at
`

// GetActiveByCustomerType returns the active restriction of a given type.
func GetActiveByCustomerType[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	restrictionType model.CustomerRestrictionType,
) (model.CustomerRestriction, error) {
	const op = "customerrestrictionrp.GetActiveByCustomerType"

	if customerID.IsZero() {
		return model.CustomerRestriction{}, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	if !restrictionType.IsValid() {
		return model.CustomerRestriction{}, cerr.InternalError(fmt.Errorf("%s: restriction type is invalid", op))
	}

	const query = `SELECT` + restrictionColumns + `
FROM customer_restrictions
WHERE customer_id = $1
  AND restriction_type = $2
  AND status = $3
`

	return postgres.QueryOne[model.CustomerRestriction](
		ctx,
		q,
		"get active customer restriction by type",
		query,
		customerID,
		restrictionType,
		model.ActiveCustomerRestrictionStatus,
	)
}

// ListActiveByCustomerID returns all active restrictions for a customer.
func ListActiveByCustomerID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
) ([]model.CustomerRestriction, error) {
	const op = "customerrestrictionrp.ListActiveByCustomerID"

	if customerID.IsZero() {
		return nil, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	const query = `SELECT` + restrictionColumns + `
FROM customer_restrictions
WHERE customer_id = $1
  AND status = $2
ORDER BY applied_at ASC, id ASC
`

	return postgres.QueryAll[model.CustomerRestriction](
		ctx,
		q,
		"list active customer restrictions",
		query,
		customerID,
		model.ActiveCustomerRestrictionStatus,
	)
}

// LockActiveByCustomerType locks the active restriction of a given type.
func LockActiveByCustomerType[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	restrictionType model.CustomerRestrictionType,
) (model.CustomerRestriction, error) {
	const op = "customerrestrictionrp.LockActiveByCustomerType"

	if customerID.IsZero() {
		return model.CustomerRestriction{}, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	if !restrictionType.IsValid() {
		return model.CustomerRestriction{}, cerr.InternalError(fmt.Errorf("%s: restriction type is invalid", op))
	}

	const query = `SELECT` + restrictionColumns + `
FROM customer_restrictions
WHERE customer_id = $1
  AND restriction_type = $2
  AND status = $3
FOR UPDATE
`

	return postgres.QueryOne[model.CustomerRestriction](
		ctx,
		q,
		"lock active customer restriction by type",
		query,
		customerID,
		restrictionType,
		model.ActiveCustomerRestrictionStatus,
	)
}

// Create inserts a new active customer restriction.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	message repo.CreateCustomerRestriction,
) (model.CustomerRestriction, error) {
	const op = "customerrestrictionrp.Create"

	if message.ID.IsZero() {
		return model.CustomerRestriction{}, cerr.InternalError(fmt.Errorf("%s: restriction id is required", op))
	}

	if message.CustomerID.IsZero() {
		return model.CustomerRestriction{}, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	if !message.RestrictionType.IsValid() {
		return model.CustomerRestriction{}, cerr.InternalError(fmt.Errorf("%s: restriction type is invalid", op))
	}

	if !message.Reason.IsValid() {
		return model.CustomerRestriction{}, cerr.InternalError(fmt.Errorf("%s: restriction reason is invalid", op))
	}

	if message.OpeningDecisionID.IsZero() {
		return model.CustomerRestriction{}, cerr.InternalError(fmt.Errorf("%s: opening decision id is required", op))
	}

	if err := postgres.RequiredTimes(map[string]time.Time{
		"applied_at": message.AppliedAt,
		"created_at": message.CreatedAt,
		"updated_at": message.UpdatedAt,
	}); err != nil {
		return model.CustomerRestriction{}, err
	}

	const query = `
INSERT INTO customer_restrictions (
	id,
	customer_id,
	restriction_type,
	reason,
	status,
	default_episode_id,
	opening_decision_id,
	releasing_decision_id,
	applied_at,
	released_at,
	created_at,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, NULL, $8, NULL, $9, $10
)
RETURNING` + restrictionColumns

	row, err := postgres.QueryOne[model.CustomerRestriction](
		ctx,
		q,
		"create customer restriction",
		query,
		message.ID,
		message.CustomerID,
		message.RestrictionType,
		message.Reason,
		model.ActiveCustomerRestrictionStatus,
		postgres.Nullable(message.DefaultEpisodeID),
		message.OpeningDecisionID,
		message.AppliedAt.UTC(),
		message.CreatedAt.UTC(),
		message.UpdatedAt.UTC(),
	)
	if err != nil {
		if postgres.IsUniqueViolation(err, "customer_restrictions_one_active_type_idx") {
			return model.CustomerRestriction{}, cerr.Conflict(fmt.Errorf("%s: active restriction already exists", op))
		}

		return model.CustomerRestriction{}, err
	}

	return row, nil
}

// Release marks an active restriction as released without deleting it.
func Release[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CustomerRestrictionID,
	releasingDecisionID model.DecisionID,
	releasedAt time.Time,
	updatedAt time.Time,
) (model.CustomerRestriction, error) {
	const op = "customerrestrictionrp.Release"

	if id.IsZero() {
		return model.CustomerRestriction{}, cerr.InternalError(fmt.Errorf("%s: restriction id is required", op))
	}

	if releasingDecisionID.IsZero() {
		return model.CustomerRestriction{}, cerr.InternalError(fmt.Errorf("%s: releasing decision id is required", op))
	}

	if err := postgres.RequiredTimes(map[string]time.Time{
		"released_at": releasedAt,
		"updated_at":  updatedAt,
	}); err != nil {
		return model.CustomerRestriction{}, err
	}

	const query = `
UPDATE customer_restrictions
SET status = $2,
    releasing_decision_id = $3,
    released_at = $4,
    updated_at = $5
WHERE id = $1
  AND status = $6
RETURNING` + restrictionColumns

	return postgres.QueryOne[model.CustomerRestriction](
		ctx,
		q,
		"release customer restriction",
		query,
		id,
		model.ReleasedCustomerRestrictionStatus,
		releasingDecisionID,
		releasedAt.UTC(),
		updatedAt.UTC(),
		model.ActiveCustomerRestrictionStatus,
	)
}

func LockActiveByDefaultEpisodeID[Q postgres.Queryer](ctx context.Context, q Q, customer model.CustomerID, episode model.DefaultEpisodeID) (model.CustomerRestriction, error) {
	return postgres.QueryOne[model.CustomerRestriction](ctx, q, "lock episode restriction", `SELECT`+restrictionColumns+` FROM customer_restrictions WHERE customer_id=$1 AND default_episode_id=$2 AND restriction_type='new_credit_usage' AND reason='collections_default' AND status='active' FOR UPDATE`, customer, episode)
}
