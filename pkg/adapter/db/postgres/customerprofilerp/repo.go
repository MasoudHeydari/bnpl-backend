package customerprofilerp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core customer profile repository contract.
var _ repo.CustomerProfile = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the customer profile repository.
// It is stateless and safe for concurrent reuse.
type Repo struct{}

// New returns a stateless PostgreSQL customer profile repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.CustomerProfileConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.CustomerProfileTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

// Verify connQueryer implements the core customer profile connection queryer contract.
var _ repo.CustomerProfileConnQueryer = (*connQueryer)(nil)

// GetByID returns the customer profile identified by id.
func (q connQueryer) GetByID(ctx context.Context, id model.CustomerID) (model.CustomerProfile, error) {
	return GetByID(ctx, q.Conn, id)
}

// ExistsByIdentitySubjectID reports whether a customer identity already has a BNPL profile.
func (q connQueryer) ExistsByIdentitySubjectID(ctx context.Context, identitySubjectID model.IdentitySubjectID) (bool, error) {
	return ExistsByIdentitySubjectID(ctx, q.Conn, identitySubjectID)
}

// GetByIdentitySubjectID returns the BNPL profile for a customer identity.
func (q connQueryer) GetByIdentitySubjectID(ctx context.Context, identitySubjectID model.IdentitySubjectID) (model.CustomerProfile, error) {
	return GetByIdentitySubjectID(ctx, q.Conn, identitySubjectID)
}

type txQueryer struct {
	*postgres.Tx
}

// Verify txQueryer implements the core customer profile transaction queryer contract.
var _ repo.CustomerProfileTxQueryer = (*txQueryer)(nil)

// Create stores a new customer profile inside the current transaction.
func (q txQueryer) Create(
	ctx context.Context,
	identitySubjectID model.IdentitySubjectID,
	jurisdiction model.Jurisdiction,
	status model.CustomerStatus,
	createdAt time.Time,
	updatedAt time.Time,
) (model.CustomerProfile, error) {
	return Create(
		ctx,
		q.Tx,
		identitySubjectID,
		jurisdiction,
		status,
		createdAt,
		updatedAt,
	)
}

// EnsureExists inserts a customer profile when missing and locks the row.
func (q txQueryer) EnsureExists(
	ctx context.Context,
	identitySubjectID model.IdentitySubjectID,
	jurisdiction model.Jurisdiction,
	status model.CustomerStatus,
	createdAt time.Time,
	updatedAt time.Time,
) (model.CustomerProfile, bool, error) {
	return EnsureExists(ctx, q.Tx, identitySubjectID, jurisdiction, status, createdAt, updatedAt)
}

// LockByID returns the customer profile and locks it for state changes.
func (q txQueryer) LockByID(ctx context.Context, id model.CustomerID) (model.CustomerProfile, error) {
	return LockByID(ctx, q.Tx, id)
}

// LockByIdentitySubjectID returns the customer profile for identitySubjectID and locks it for state changes.
func (q txQueryer) LockByIdentitySubjectID(ctx context.Context, identitySubjectID model.IdentitySubjectID) (model.CustomerProfile, error) {
	return LockByIdentitySubjectID(ctx, q.Tx, identitySubjectID)
}

// Update persists the full customer profile state and returns the updated row.
func (q txQueryer) Update(
	ctx context.Context,
	id model.CustomerID,
	identitySubjectID model.IdentitySubjectID,
	jurisdiction model.Jurisdiction,
	status model.CustomerStatus,
	updatedAt time.Time,
) (model.CustomerProfile, error) {
	return Update(
		ctx,
		q.Tx,
		id,
		identitySubjectID,
		jurisdiction,
		status,
		updatedAt,
	)
}
