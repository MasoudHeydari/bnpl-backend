package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreateDefaultEpisode carries fields required to open a default episode.
type CreateDefaultEpisode struct {
	ID                model.DefaultEpisodeID
	CustomerID        model.CustomerID
	OpeningDecisionID model.DecisionID
	StartedAt         time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Triggers          []model.DefaultEpisodeTrigger
}

// DefaultEpisodeQueryer defines read-only default episode operations.
type DefaultEpisodeQueryer interface {
	GetActiveByCustomerID(
		ctx context.Context,
		customerID model.CustomerID,
	) (model.DefaultEpisode, error)

	GetByID(ctx context.Context, id model.DefaultEpisodeID) (model.DefaultEpisode, error)

	ListTriggersByEpisodeID(
		ctx context.Context,
		episodeID model.DefaultEpisodeID,
	) ([]model.DefaultEpisodeTrigger, error)
}

// DefaultEpisodeConnQueryer exposes default episode operations on a connection.
type DefaultEpisodeConnQueryer interface {
	DefaultEpisodeQueryer
}

// DefaultEpisodeTxQueryer exposes transaction-scoped default episode operations.
type DefaultEpisodeTxQueryer interface {
	DefaultEpisodeQueryer

	LockActiveByCustomerID(
		ctx context.Context,
		customerID model.CustomerID,
	) (model.DefaultEpisode, error)

	Create(ctx context.Context, message CreateDefaultEpisode) (model.DefaultEpisode, error)

	MarkRecovered(
		ctx context.Context,
		episodeID model.DefaultEpisodeID,
		closingDecisionID model.DecisionID,
		recoveredAt time.Time,
		updatedAt time.Time,
	) (model.DefaultEpisode, error)
}

// DefaultEpisode defines repository bindings for default episodes.
type DefaultEpisode interface {
	Conn(conn Conn) DefaultEpisodeConnQueryer
	Tx(tx Tx) DefaultEpisodeTxQueryer
}
