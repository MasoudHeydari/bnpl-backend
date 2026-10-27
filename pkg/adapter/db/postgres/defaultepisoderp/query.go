package defaultepisoderp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const episodeColumns = `
	id,
	customer_id,
	status,
	opening_decision_id,
	closing_decision_id,
	started_at,
	recovered_at,
	created_at,
	updated_at
`

const triggerColumns = `
	episode_id,
	installment_id,
	due_at,
	grace_period_days,
	overdue_eligible_at
`

// GetActiveByCustomerID returns the active default episode for a customer.
func GetActiveByCustomerID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
) (model.DefaultEpisode, error) {
	const op = "defaultepisoderp.GetActiveByCustomerID"

	if customerID.IsZero() {
		return model.DefaultEpisode{}, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	const query = `SELECT` + episodeColumns + `
FROM default_episodes
WHERE customer_id = $1
  AND status = $2
`

	return postgres.QueryOne[model.DefaultEpisode](
		ctx,
		q,
		"get active default episode by customer id",
		query,
		customerID,
		model.ActiveDefaultEpisodeStatus,
	)
}

// GetByID returns a default episode by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.DefaultEpisodeID,
) (model.DefaultEpisode, error) {
	const op = "defaultepisoderp.GetByID"

	if id.IsZero() {
		return model.DefaultEpisode{}, cerr.InternalError(fmt.Errorf("%s: episode id is required", op))
	}

	const query = `SELECT` + episodeColumns + `
FROM default_episodes
WHERE id = $1
`

	return postgres.QueryOne[model.DefaultEpisode](
		ctx,
		q,
		"get default episode by id",
		query,
		id,
	)
}

// LockActiveByCustomerID locks the active episode for a customer, if any.
func LockActiveByCustomerID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
) (model.DefaultEpisode, error) {
	const op = "defaultepisoderp.LockActiveByCustomerID"

	if customerID.IsZero() {
		return model.DefaultEpisode{}, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	const query = `SELECT` + episodeColumns + `
FROM default_episodes
WHERE customer_id = $1
  AND status = $2
FOR UPDATE
`

	return postgres.QueryOne[model.DefaultEpisode](
		ctx,
		q,
		"lock active default episode by customer id",
		query,
		customerID,
		model.ActiveDefaultEpisodeStatus,
	)
}

// Create inserts a new active default episode and its trigger evidence.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	message repo.CreateDefaultEpisode,
) (model.DefaultEpisode, error) {
	const op = "defaultepisoderp.Create"

	if message.ID.IsZero() {
		return model.DefaultEpisode{}, cerr.InternalError(fmt.Errorf("%s: episode id is required", op))
	}

	if message.CustomerID.IsZero() {
		return model.DefaultEpisode{}, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	if message.OpeningDecisionID.IsZero() {
		return model.DefaultEpisode{}, cerr.InternalError(fmt.Errorf("%s: opening decision id is required", op))
	}

	if len(message.Triggers) == 0 {
		return model.DefaultEpisode{}, cerr.InternalError(fmt.Errorf("%s: at least one trigger is required", op))
	}

	if err := postgres.RequiredTimes(map[string]time.Time{
		"started_at": message.StartedAt,
		"created_at": message.CreatedAt,
		"updated_at": message.UpdatedAt,
	}); err != nil {
		return model.DefaultEpisode{}, err
	}

	const insertEpisode = `
INSERT INTO default_episodes (
	id,
	customer_id,
	status,
	opening_decision_id,
	closing_decision_id,
	started_at,
	recovered_at,
	created_at,
	updated_at
) VALUES (
	$1, $2, $3, $4, NULL, $5, NULL, $6, $7
)
RETURNING` + episodeColumns

	episode, err := postgres.QueryOne[model.DefaultEpisode](
		ctx,
		q,
		"create default episode",
		insertEpisode,
		message.ID,
		message.CustomerID,
		model.ActiveDefaultEpisodeStatus,
		message.OpeningDecisionID,
		message.StartedAt.UTC(),
		message.CreatedAt.UTC(),
		message.UpdatedAt.UTC(),
	)
	if err != nil {
		if postgres.IsUniqueViolation(err, "default_episodes_one_active_per_customer_idx") {
			return model.DefaultEpisode{}, cerr.Conflict(fmt.Errorf("%s: active episode already exists", op))
		}

		return model.DefaultEpisode{}, err
	}

	const insertTrigger = `
INSERT INTO default_episode_triggers (
	episode_id,
	installment_id,
	due_at,
	grace_period_days,
	overdue_eligible_at
) VALUES (
	$1, $2, $3, $4, $5
)
`

	for _, trigger := range message.Triggers {
		if trigger.InstallmentID.IsZero() {
			return model.DefaultEpisode{}, cerr.InternalError(fmt.Errorf("%s: trigger installment id is required", op))
		}

		_, execErr := q.Exec(
			ctx,
			insertTrigger,
			episode.ID,
			trigger.InstallmentID,
			trigger.DueAt.UTC(),
			int(trigger.GracePeriodDays),
			trigger.OverdueEligibleAt.UTC(),
		)
		if execErr != nil {
			return model.DefaultEpisode{}, cerr.InternalError(fmt.Errorf(
				"%s: insert trigger: %w",
				op,
				postgres.MapError(execErr),
			))
		}
	}

	return episode, nil
}

// MarkRecovered closes an active episode.
func MarkRecovered[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	episodeID model.DefaultEpisodeID,
	closingDecisionID model.DecisionID,
	recoveredAt time.Time,
	updatedAt time.Time,
) (model.DefaultEpisode, error) {
	const op = "defaultepisoderp.MarkRecovered"

	if episodeID.IsZero() {
		return model.DefaultEpisode{}, cerr.InternalError(fmt.Errorf("%s: episode id is required", op))
	}

	if closingDecisionID.IsZero() {
		return model.DefaultEpisode{}, cerr.InternalError(fmt.Errorf("%s: closing decision id is required", op))
	}

	if err := postgres.RequiredTimes(map[string]time.Time{
		"recovered_at": recoveredAt,
		"updated_at":   updatedAt,
	}); err != nil {
		return model.DefaultEpisode{}, err
	}

	const query = `
UPDATE default_episodes
SET status = $2,
    closing_decision_id = $3,
    recovered_at = $4,
    updated_at = $5
WHERE id = $1
  AND status = $6
RETURNING` + episodeColumns

	return postgres.QueryOne[model.DefaultEpisode](
		ctx,
		q,
		"mark default episode recovered",
		query,
		episodeID,
		model.RecoveredDefaultEpisodeStatus,
		closingDecisionID,
		recoveredAt.UTC(),
		updatedAt.UTC(),
		model.ActiveDefaultEpisodeStatus,
	)
}

// ListTriggersByEpisodeID returns trigger evidence for an episode.
func ListTriggersByEpisodeID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	episodeID model.DefaultEpisodeID,
) ([]model.DefaultEpisodeTrigger, error) {
	const op = "defaultepisoderp.ListTriggersByEpisodeID"

	if episodeID.IsZero() {
		return nil, cerr.InternalError(fmt.Errorf("%s: episode id is required", op))
	}

	const query = `SELECT` + triggerColumns + `
FROM default_episode_triggers
WHERE episode_id = $1
ORDER BY overdue_eligible_at ASC, installment_id ASC
`

	return postgres.QueryAll[model.DefaultEpisodeTrigger](
		ctx,
		q,
		"list default episode triggers",
		query,
		episodeID,
	)
}
