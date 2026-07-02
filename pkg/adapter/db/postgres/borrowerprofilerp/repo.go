package borrowerprofilerp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core borrower profile repository contract.
var _ repo.BorrowerProfile = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the borrower profile repository.
// It is stateless and safe for concurrent reuse.
type Repo struct{}

// New returns a stateless PostgreSQL borrower profile repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.BorrowerProfileConnQueryer {
	return connQueryer{Conn: c.(*postgres.Conn)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.BorrowerProfileTxQueryer {
	return txQueryer{Tx: tx.(*postgres.Tx)}
}

type connQueryer struct {
	*postgres.Conn
}

// Verify connQueryer implements the core borrower profile connection queryer contract.
var _ repo.BorrowerProfileConnQueryer = (*connQueryer)(nil)

// GetByID returns the borrower profile identified by id.
func (q connQueryer) GetByID(ctx context.Context, id uuid.UUID) (model.BorrowerProfile, error) {
	return GetByID(ctx, q.Conn, id)
}

// ExistsByBorrowerID reports whether a borrower identity already has a BNPL profile.
func (q connQueryer) ExistsByBorrowerID(ctx context.Context, borrowerID uuid.UUID) (bool, error) {
	return ExistsByBorrowerID(ctx, q.Conn, borrowerID)
}

// GetByBorrowerID returns the BNPL profile for a borrower identity.
func (q connQueryer) GetByBorrowerID(ctx context.Context, borrowerID uuid.UUID) (model.BorrowerProfile, error) {
	return GetByBorrowerID(ctx, q.Conn, borrowerID)
}

type txQueryer struct {
	*postgres.Tx
}

// Verify txQueryer implements the core borrower profile transaction queryer contract.
var _ repo.BorrowerProfileTxQueryer = (*txQueryer)(nil)

// Create stores a new borrower profile inside the current transaction.
func (q txQueryer) Create(
	ctx context.Context,
	borrowerID uuid.UUID,
	identityVerificationID *uuid.UUID,
	lastProfileEligibilityDecisionID *uuid.UUID,
	lastCreditLimitID *uuid.UUID,
	jurisdiction model.Jurisdiction,
	status model.BorrowerStatus,
	createdAt time.Time,
	updatedAt time.Time,
) (model.BorrowerProfile, error) {
	return Create(
		ctx,
		q.Tx,
		borrowerID,
		identityVerificationID,
		lastProfileEligibilityDecisionID,
		lastCreditLimitID,
		jurisdiction,
		status,
		createdAt,
		updatedAt,
	)
}

// LockByID returns the borrower profile and locks it for state changes.
func (q txQueryer) LockByID(ctx context.Context, id uuid.UUID) (model.BorrowerProfile, error) {
	return LockByID(ctx, q.Tx, id)
}

// LockByBorrowerID returns the borrower profile for borrowerID and locks it for state changes.
func (q txQueryer) LockByBorrowerID(ctx context.Context, borrowerID uuid.UUID) (model.BorrowerProfile, error) {
	return LockByBorrowerID(ctx, q.Tx, borrowerID)
}

// Update persists the full borrower profile state.
func (q txQueryer) Update(
	ctx context.Context,
	id uuid.UUID,
	borrowerID uuid.UUID,
	identityVerificationID *uuid.UUID,
	lastProfileEligibilityDecisionID *uuid.UUID,
	lastCreditLimitID *uuid.UUID,
	jurisdiction model.Jurisdiction,
	status model.BorrowerStatus,
	createdAt time.Time,
	updatedAt time.Time,
) error {
	return Update(
		ctx,
		q.Tx,
		id,
		borrowerID,
		identityVerificationID,
		lastProfileEligibilityDecisionID,
		lastCreditLimitID,
		jurisdiction,
		status,
		createdAt,
		updatedAt,
	)
}

// SetIdentityVerificationID records the latest identity verification artifact.
func (q txQueryer) SetIdentityVerificationID(ctx context.Context, profileID uuid.UUID, identityVerificationID uuid.UUID, updatedAt time.Time) error {
	return SetIdentityVerificationID(ctx, q.Tx, profileID, identityVerificationID, updatedAt)
}

// SetLastProfileEligibilityDecisionID records the latest profile-level eligibility decision.
func (q txQueryer) SetLastProfileEligibilityDecisionID(ctx context.Context, profileID uuid.UUID, decisionID uuid.UUID, updatedAt time.Time) error {
	return SetLastProfileEligibilityDecisionID(ctx, q.Tx, profileID, decisionID, updatedAt)
}

// SetLastCreditLimitID records the current credit limit shortcut for the profile.
func (q txQueryer) SetLastCreditLimitID(ctx context.Context, profileID uuid.UUID, creditLimitID uuid.UUID, updatedAt time.Time) error {
	return SetLastCreditLimitID(ctx, q.Tx, profileID, creditLimitID, updatedAt)
}
