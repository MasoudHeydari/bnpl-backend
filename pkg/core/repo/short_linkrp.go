package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ShortLinkQueryer defines read-only short-link operations.
type ShortLinkQueryer interface {
	// GetByCode returns a short link by its public code.
	GetByCode(ctx context.Context, code string) (model.ShortLink, error)

	// GetByID returns a short link by id.
	GetByID(ctx context.Context, id model.ShortLinkID) (model.ShortLink, error)
}

// ShortLinkConnQueryer exposes short-link operations on a connection.
type ShortLinkConnQueryer interface {
	// ShortLinkQueryer provides shared short-link query operations.
	ShortLinkQueryer
}

// ShortLinkTxQueryer exposes transaction-scoped short-link operations.
type ShortLinkTxQueryer interface {
	// ShortLinkQueryer provides shared short-link query operations.
	ShortLinkQueryer

	// Create stores a new short link. Callers must handle unique-code collisions.
	Create(
		ctx context.Context,
		id model.ShortLinkID,
		code string,
		destinationURL string,
		expiresAt *time.Time,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.ShortLink, error)

	// EnsureAlias upserts a stable alias code to the given destination.
	// When the code already exists with the same destination, the existing row
	// is returned unchanged (aside from refreshed updated_at when destination
	// changes). Destination changes update the row in place.
	EnsureAlias(
		ctx context.Context,
		id model.ShortLinkID,
		code string,
		destinationURL string,
		now time.Time,
	) (model.ShortLink, error)

	// Revoke marks a short link revoked when it is not already revoked.
	Revoke(
		ctx context.Context,
		code string,
		revokedAt time.Time,
	) (model.ShortLink, error)
}

// ShortLink defines repository bindings for opaque URL short links.
type ShortLink interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(conn Conn) ShortLinkConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(tx Tx) ShortLinkTxQueryer
}
