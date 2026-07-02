package borrowercreditlimitrp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core borrower credit limit repository contract.
var _ repo.BorrowerCreditLimit = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the borrower credit limit repository.
// It is stateless and safe for concurrent reuse.
type Repo struct{}

// New returns a stateless PostgreSQL borrower credit limit repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.BorrowerCreditLimitConnQueryer {
	return connQueryer{Conn: c.(*postgres.Conn)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.BorrowerCreditLimitTxQueryer {
	return txQueryer{Tx: tx.(*postgres.Tx)}
}

type connQueryer struct {
	*postgres.Conn
}

// Verify connQueryer implements the core borrower credit limit connection queryer contract.
var _ repo.BorrowerCreditLimitConnQueryer = (*connQueryer)(nil)

// GetByID returns the credit limit identified by id.
func (q connQueryer) GetByID(ctx context.Context, id uuid.UUID) (model.BorrowerCreditLimit, error) {
	return GetByID(ctx, q.Conn, id)
}

// GetCurrentByBorrowerProfileID returns the active credit limit at a point in time.
func (q connQueryer) GetCurrentByBorrowerProfileID(ctx context.Context, borrowerProfileID uuid.UUID, at time.Time) (model.BorrowerCreditLimit, error) {
	return GetCurrentByBorrowerProfileID(ctx, q.Conn, borrowerProfileID, at)
}

// ListByBorrowerProfileID returns all credit limits for a borrower profile.
func (q connQueryer) ListByBorrowerProfileID(ctx context.Context, borrowerProfileID uuid.UUID) ([]model.BorrowerCreditLimit, error) {
	return ListByBorrowerProfileID(ctx, q.Conn, borrowerProfileID)
}

type txQueryer struct {
	*postgres.Tx
}

// Verify txQueryer implements the core borrower credit limit transaction queryer contract.
var _ repo.BorrowerCreditLimitTxQueryer = (*txQueryer)(nil)

// Create stores a new borrower credit limit inside the current transaction.
func (q txQueryer) Create(
	ctx context.Context,
	borrowerProfileID uuid.UUID,
	decisionID uuid.UUID,
	amountMinor uint64,
	currency model.Currency,
	status model.BorrowerCreditLimitStatus,
	effectiveFrom time.Time,
	expiresAt *time.Time,
	revokedAt *time.Time,
	replacedByCreditLimitID *uuid.UUID,
	createdAt time.Time,
	updatedAt time.Time,
) (model.BorrowerCreditLimit, error) {
	return Create(
		ctx,
		q.Tx,
		borrowerProfileID,
		decisionID,
		amountMinor,
		currency,
		status,
		effectiveFrom,
		expiresAt,
		revokedAt,
		replacedByCreditLimitID,
		createdAt,
		updatedAt,
	)
}

// LockCurrentByBorrowerProfileID returns and locks the active credit limit at a point in time.
func (q txQueryer) LockCurrentByBorrowerProfileID(ctx context.Context, borrowerProfileID uuid.UUID, at time.Time) (model.BorrowerCreditLimit, error) {
	return LockCurrentByBorrowerProfileID(ctx, q.Tx, borrowerProfileID, at)
}

// Revoke marks a credit limit as revoked by a later governed decision.
func (q txQueryer) Revoke(ctx context.Context, id uuid.UUID, revokedAt time.Time, updatedAt time.Time) error {
	return Revoke(ctx, q.Tx, id, revokedAt, updatedAt)
}

// MarkExpired marks a credit limit as expired.
func (q txQueryer) MarkExpired(ctx context.Context, id uuid.UUID, updatedAt time.Time) error {
	return MarkExpired(ctx, q.Tx, id, updatedAt)
}

// MarkReplaced links a credit limit to the newer assignment that replaced it.
func (q txQueryer) MarkReplaced(ctx context.Context, id uuid.UUID, replacedByCreditLimitID uuid.UUID, updatedAt time.Time) error {
	return MarkReplaced(ctx, q.Tx, id, replacedByCreditLimitID, updatedAt)
}
