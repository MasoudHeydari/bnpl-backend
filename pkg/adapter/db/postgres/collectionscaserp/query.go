package collectionscaserp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const caseColumns = `
	id,
	customer_id,
	default_episode_id,
	opening_decision_id,
	status,
	created_at,
	updated_at,
	resolved_at
`

const eventColumns = `
	id,
	case_id,
	event_type,
	decision_id,
	payload,
	occurred_at,
	actor_type,
	actor_id,
	created_at
`

// GetByDefaultEpisodeID returns the collections case for an episode.
func GetByDefaultEpisodeID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	episodeID model.DefaultEpisodeID,
) (model.CollectionsCase, error) {
	const op = "collectionscaserp.GetByDefaultEpisodeID"

	if episodeID.IsZero() {
		return model.CollectionsCase{}, cerr.InternalError(fmt.Errorf("%s: episode id is required", op))
	}

	const query = `SELECT` + caseColumns + `
FROM collections_cases
WHERE default_episode_id = $1
`

	return postgres.QueryOne[model.CollectionsCase](
		ctx,
		q,
		"get collections case by default episode id",
		query,
		episodeID,
	)
}

// GetByID returns a collections case by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CollectionsCaseID,
) (model.CollectionsCase, error) {
	const op = "collectionscaserp.GetByID"

	if id.IsZero() {
		return model.CollectionsCase{}, cerr.InternalError(fmt.Errorf("%s: case id is required", op))
	}

	const query = `SELECT` + caseColumns + `
FROM collections_cases
WHERE id = $1
`

	return postgres.QueryOne[model.CollectionsCase](
		ctx,
		q,
		"get collections case by id",
		query,
		id,
	)
}

// Create inserts a new collections case.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	message repo.CreateCollectionsCase,
) (model.CollectionsCase, error) {
	const op = "collectionscaserp.Create"

	if message.ID.IsZero() {
		return model.CollectionsCase{}, cerr.InternalError(fmt.Errorf("%s: case id is required", op))
	}

	if message.CustomerID.IsZero() {
		return model.CollectionsCase{}, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	if message.DefaultEpisodeID.IsZero() {
		return model.CollectionsCase{}, cerr.InternalError(fmt.Errorf("%s: default episode id is required", op))
	}

	if message.OpeningDecisionID.IsZero() {
		return model.CollectionsCase{}, cerr.InternalError(fmt.Errorf("%s: opening decision id is required", op))
	}

	if err := postgres.RequiredTimes(map[string]time.Time{
		"created_at": message.CreatedAt,
		"updated_at": message.UpdatedAt,
	}); err != nil {
		return model.CollectionsCase{}, err
	}

	const query = `
INSERT INTO collections_cases (
	id,
	customer_id,
	default_episode_id,
	opening_decision_id,
	status,
	created_at,
	updated_at,
	resolved_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, NULL
)
RETURNING` + caseColumns

	row, err := postgres.QueryOne[model.CollectionsCase](
		ctx,
		q,
		"create collections case",
		query,
		message.ID,
		message.CustomerID,
		message.DefaultEpisodeID,
		message.OpeningDecisionID,
		model.PendingHumanReviewCollectionsCaseStatus,
		message.CreatedAt.UTC(),
		message.UpdatedAt.UTC(),
	)
	if err != nil {
		if postgres.IsUniqueViolation(err, "collections_cases_episode_unique") {
			return model.CollectionsCase{}, cerr.Conflict(fmt.Errorf("%s: case already exists for episode", op))
		}

		return model.CollectionsCase{}, err
	}

	return row, nil
}

// LatestEventOfType returns the newest event of the given type for a case.
func LatestEventOfType[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	caseID model.CollectionsCaseID,
	eventType model.CollectionsCaseEventType,
) (model.CollectionsCaseEvent, error) {
	const op = "collectionscaserp.LatestEventOfType"

	if caseID.IsZero() {
		return model.CollectionsCaseEvent{}, cerr.InternalError(fmt.Errorf("%s: case id is required", op))
	}

	if eventType == "" {
		return model.CollectionsCaseEvent{}, cerr.InternalError(fmt.Errorf("%s: event type is required", op))
	}

	const query = `SELECT` + eventColumns + `
FROM collections_case_events
WHERE case_id = $1
  AND event_type = $2
ORDER BY occurred_at DESC, id DESC
LIMIT 1
`

	return postgres.QueryOne[model.CollectionsCaseEvent](
		ctx,
		q,
		"get latest collections case event by type",
		query,
		caseID,
		eventType,
	)
}

// AppendEvent inserts an append-only case event.
func AppendEvent[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	message repo.CreateCollectionsCaseEvent,
) (model.CollectionsCaseEvent, error) {
	const op = "collectionscaserp.AppendEvent"

	if message.ID.IsZero() {
		return model.CollectionsCaseEvent{}, cerr.InternalError(fmt.Errorf("%s: event id is required", op))
	}

	if message.CaseID.IsZero() {
		return model.CollectionsCaseEvent{}, cerr.InternalError(fmt.Errorf("%s: case id is required", op))
	}

	if message.EventType == "" {
		return model.CollectionsCaseEvent{}, cerr.InternalError(fmt.Errorf("%s: event type is required", op))
	}

	if !message.ActorType.IsValid() {
		return model.CollectionsCaseEvent{}, cerr.InternalError(fmt.Errorf("%s: actor type is invalid", op))
	}

	if err := postgres.RequiredTimes(map[string]time.Time{
		"occurred_at": message.OccurredAt,
		"created_at":  message.CreatedAt,
	}); err != nil {
		return model.CollectionsCaseEvent{}, err
	}

	const query = `
INSERT INTO collections_case_events (
	id,
	case_id,
	event_type,
	decision_id,
	payload,
	occurred_at,
	actor_type,
	actor_id,
	created_at
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING` + eventColumns

	return postgres.QueryOne[model.CollectionsCaseEvent](
		ctx,
		q,
		"append collections case event",
		query,
		message.ID,
		message.CaseID,
		message.EventType,
		postgres.Nullable(message.DecisionID),
		message.Payload,
		message.OccurredAt.UTC(),
		message.ActorType,
		postgres.Nullable(message.ActorID),
		message.CreatedAt.UTC(),
	)
}
