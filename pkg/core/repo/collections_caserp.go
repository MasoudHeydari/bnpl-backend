package repo

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreateCollectionsCase carries fields required to open a collections case.
type CreateCollectionsCase struct {
	ID                model.CollectionsCaseID
	CustomerID        model.CustomerID
	DefaultEpisodeID  model.DefaultEpisodeID
	OpeningDecisionID model.DecisionID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// CreateCollectionsCaseEvent carries an append-only case event.
type CreateCollectionsCaseEvent struct {
	ID         model.CollectionsCaseEventID
	CaseID     model.CollectionsCaseID
	EventType  model.CollectionsCaseEventType
	DecisionID *model.DecisionID
	Payload    []byte
	OccurredAt time.Time
	ActorType  model.DecisionActorType
	ActorID    *uuid.UUID
	CreatedAt  time.Time
}

// CollectionsCaseQueryer defines read-only collections case operations.
type CollectionsCaseQueryer interface {
	GetByDefaultEpisodeID(
		ctx context.Context,
		episodeID model.DefaultEpisodeID,
	) (model.CollectionsCase, error)

	GetByID(ctx context.Context, id model.CollectionsCaseID) (model.CollectionsCase, error)
}

// CollectionsCaseConnQueryer exposes case operations on a connection.
type CollectionsCaseConnQueryer interface {
	CollectionsCaseQueryer
}

// CollectionsCaseTxQueryer exposes transaction-scoped case operations.
type CollectionsCaseTxQueryer interface {
	CollectionsCaseQueryer

	Create(ctx context.Context, message CreateCollectionsCase) (model.CollectionsCase, error)

	AppendEvent(
		ctx context.Context,
		message CreateCollectionsCaseEvent,
	) (model.CollectionsCaseEvent, error)
}

// CollectionsCase defines repository bindings for collections cases.
type CollectionsCase interface {
	Conn(conn Conn) CollectionsCaseConnQueryer
	Tx(tx Tx) CollectionsCaseTxQueryer
}
