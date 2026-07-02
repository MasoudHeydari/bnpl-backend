package borroweridentityverificationrp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core borrower identity verification repository contract.
var _ repo.BorrowerIdentityVerification = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the identity verification repository.
// It is stateless and safe for concurrent reuse.
type Repo struct{}

// New returns a stateless PostgreSQL identity verification repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.BorrowerIdentityVerificationConnQueryer {
	return connQueryer{Conn: c.(*postgres.Conn)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.BorrowerIdentityVerificationTxQueryer {
	return txQueryer{Tx: tx.(*postgres.Tx)}
}

type connQueryer struct {
	*postgres.Conn
}

// Verify connQueryer implements the core identity verification connection queryer contract.
var _ repo.BorrowerIdentityVerificationConnQueryer = (*connQueryer)(nil)

// GetByID returns the verification artifact identified by id.
func (q connQueryer) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (model.BorrowerIdentityVerification, error) {
	return GetByID(ctx, q.Conn, id)
}

// GetLatestByBorrowerID returns the newest verification artifact for a borrower.
func (q connQueryer) GetLatestByBorrowerID(
	ctx context.Context,
	borrowerID uuid.UUID,
) (model.BorrowerIdentityVerification, error) {
	return GetLatestByBorrowerID(ctx, q.Conn, borrowerID)
}

// FindByNationalIDHash returns a verification artifact by hashed national identifier.
func (q connQueryer) FindByNationalIDHash(
	ctx context.Context,
	nationalIDHash string,
) (model.BorrowerIdentityVerification, error) {
	return FindByNationalIDHash(ctx, q.Conn, nationalIDHash)
}

type txQueryer struct {
	*postgres.Tx
}

// Verify txQueryer implements the core identity verification transaction queryer contract.
var _ repo.BorrowerIdentityVerificationTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (model.BorrowerIdentityVerification, error) {
	return GetByID(ctx, q.Tx, id)
}

func (q txQueryer) GetLatestByBorrowerID(
	ctx context.Context,
	borrowerID uuid.UUID,
) (model.BorrowerIdentityVerification, error) {
	return GetLatestByBorrowerID(ctx, q.Tx, borrowerID)
}

func (q txQueryer) FindByNationalIDHash(
	ctx context.Context,
	nationalIDHash string,
) (model.BorrowerIdentityVerification, error) {
	return FindByNationalIDHash(ctx, q.Tx, nationalIDHash)
}

// Create stores a new verification artifact inside the current transaction.
func (q txQueryer) Create(
	ctx context.Context,
	borrowerID uuid.UUID,
	nationalIDHash string,
	phoneNumberHash string,
	birthdate time.Time,
	status model.BorrowerIdentityVerificationStatus,
	provider string,
	providerReferenceID *string,
	verifiedAt *time.Time,
	rejectedAt *time.Time,
	expiresAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.BorrowerIdentityVerification, error) {
	return Create(
		ctx,
		q.Tx,
		borrowerID,
		nationalIDHash,
		phoneNumberHash,
		birthdate,
		status,
		provider,
		providerReferenceID,
		verifiedAt,
		rejectedAt,
		expiresAt,
		createdAt,
		updatedAt,
	)
}

// LockByID returns the verification artifact and locks it for state changes.
func (q txQueryer) LockByID(
	ctx context.Context,
	id uuid.UUID,
) (model.BorrowerIdentityVerification, error) {
	return LockByID(ctx, q.Tx, id)
}

// Update persists the full verification artifact state.
func (q txQueryer) Update(
	ctx context.Context,
	id uuid.UUID,
	borrowerID uuid.UUID,
	nationalIDHash string,
	phoneNumberHash string,
	birthdate time.Time,
	status model.BorrowerIdentityVerificationStatus,
	provider string,
	providerReferenceID *string,
	verifiedAt *time.Time,
	rejectedAt *time.Time,
	expiresAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) error {
	return Update(
		ctx,
		q.Tx,
		id,
		borrowerID,
		nationalIDHash,
		phoneNumberHash,
		birthdate,
		status,
		provider,
		providerReferenceID,
		verifiedAt,
		rejectedAt,
		expiresAt,
		createdAt,
		updatedAt,
	)
}
