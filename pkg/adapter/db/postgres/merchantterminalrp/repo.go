package merchantterminalrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core merchant terminal repository contract.
var _ repo.MerchantTerminal = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the merchant terminal repository.
// It is stateless and safe for concurrent reuse.
type Repo struct{}

// New returns a stateless PostgreSQL merchant terminal repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.MerchantTerminalConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.MerchantTerminalTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

// Verify connQueryer implements the core merchant terminal connection queryer contract.
var _ repo.MerchantTerminalConnQueryer = (*connQueryer)(nil)

// GetByTerminalID returns the terminal identified by its public terminal id.
func (q connQueryer) GetByTerminalID(
	ctx context.Context,
	terminalID model.TerminalID,
) (model.MerchantTerminal, error) {
	return GetByTerminalID(ctx, q.Conn, terminalID)
}

// GetByMerchantID returns the terminal owned by merchantID.
func (q connQueryer) GetByMerchantID(
	ctx context.Context,
	merchantID model.MerchantID,
) (model.MerchantTerminal, error) {
	return GetByMerchantID(ctx, q.Conn, merchantID)
}

// GetActiveCredential returns the active credential of a terminal.
func (q connQueryer) GetActiveCredential(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
) (model.MerchantTerminalCredential, error) {
	return GetActiveCredential(ctx, q.Conn, merchantTerminalID)
}

// ListAcceptableCredentials returns the credentials accepted for authentication at now.
func (q connQueryer) ListAcceptableCredentials(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
	now time.Time,
) ([]model.MerchantTerminalCredential, error) {
	return ListAcceptableCredentials(ctx, q.Conn, merchantTerminalID, now)
}

// GetCredentialByID returns the credential identified by id.
func (q connQueryer) GetCredentialByID(
	ctx context.Context,
	id model.MerchantTerminalCredentialID,
) (model.MerchantTerminalCredential, error) {
	return GetCredentialByID(ctx, q.Conn, id)
}

// GetIssuanceDeliveryByIdempotencyKey returns the issuance delivery for a terminal and key.
func (q connQueryer) GetIssuanceDeliveryByIdempotencyKey(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
	idempotencyKey string,
) (model.MerchantTerminalIssuanceDelivery, error) {
	return GetIssuanceDeliveryByIdempotencyKey(ctx, q.Conn, merchantTerminalID, idempotencyKey)
}

// ListPermissions returns the server-side permission wire codes for a terminal.
func (q connQueryer) ListPermissions(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
) ([]string, error) {
	return ListPermissions(ctx, q.Conn, merchantTerminalID)
}

type txQueryer struct {
	*postgres.Tx
}

// Verify txQueryer implements the core merchant terminal transaction queryer contract.
var _ repo.MerchantTerminalTxQueryer = (*txQueryer)(nil)

// GetByTerminalID returns the terminal identified by its public terminal id.
func (q txQueryer) GetByTerminalID(
	ctx context.Context,
	terminalID model.TerminalID,
) (model.MerchantTerminal, error) {
	return GetByTerminalID(ctx, q.Tx, terminalID)
}

// GetByMerchantID returns the terminal owned by merchantID.
func (q txQueryer) GetByMerchantID(
	ctx context.Context,
	merchantID model.MerchantID,
) (model.MerchantTerminal, error) {
	return GetByMerchantID(ctx, q.Tx, merchantID)
}

// GetActiveCredential returns the active credential of a terminal.
func (q txQueryer) GetActiveCredential(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
) (model.MerchantTerminalCredential, error) {
	return GetActiveCredential(ctx, q.Tx, merchantTerminalID)
}

// ListAcceptableCredentials returns the credentials accepted for authentication at now.
func (q txQueryer) ListAcceptableCredentials(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
	now time.Time,
) ([]model.MerchantTerminalCredential, error) {
	return ListAcceptableCredentials(ctx, q.Tx, merchantTerminalID, now)
}

// GetCredentialByID returns the credential identified by id.
func (q txQueryer) GetCredentialByID(
	ctx context.Context,
	id model.MerchantTerminalCredentialID,
) (model.MerchantTerminalCredential, error) {
	return GetCredentialByID(ctx, q.Tx, id)
}

// GetIssuanceDeliveryByIdempotencyKey returns the issuance delivery for a terminal and key.
func (q txQueryer) GetIssuanceDeliveryByIdempotencyKey(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
	idempotencyKey string,
) (model.MerchantTerminalIssuanceDelivery, error) {
	return GetIssuanceDeliveryByIdempotencyKey(ctx, q.Tx, merchantTerminalID, idempotencyKey)
}

// ListPermissions returns the server-side permission wire codes for a terminal.
func (q txQueryer) ListPermissions(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
) ([]string, error) {
	return ListPermissions(ctx, q.Tx, merchantTerminalID)
}

// LockByTerminalID returns the terminal and locks it for state changes.
func (q txQueryer) LockByTerminalID(
	ctx context.Context,
	terminalID model.TerminalID,
) (model.MerchantTerminal, error) {
	return LockByTerminalID(ctx, q.Tx, terminalID)
}

// LockByMerchantID returns the terminal owned by merchantID and locks it for state changes.
func (q txQueryer) LockByMerchantID(
	ctx context.Context,
	merchantID model.MerchantID,
) (model.MerchantTerminal, error) {
	return LockByMerchantID(ctx, q.Tx, merchantID)
}

// CreateTerminal stores a new terminal inside the current transaction.
func (q txQueryer) CreateTerminal(
	ctx context.Context,
	merchantID model.MerchantID,
	terminalID model.TerminalID,
	status model.MerchantTerminalStatus,
	createdAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminal, error) {
	return CreateTerminal(
		ctx,
		q.Tx,
		merchantID,
		terminalID,
		status,
		createdAt,
		updatedAt,
	)
}

// ReplacePermissions replaces all permission assignments for the terminal.
func (q txQueryer) ReplacePermissions(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
	permissions []string,
	createdAt time.Time,
) error {
	return ReplacePermissions(
		ctx,
		q.Tx,
		merchantTerminalID,
		permissions,
		createdAt,
	)
}

// LockCredentialsForRotation locks and returns the active and retiring credentials of a terminal.
func (q txQueryer) LockCredentialsForRotation(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
) ([]model.MerchantTerminalCredential, error) {
	return LockCredentialsForRotation(ctx, q.Tx, merchantTerminalID)
}

// CreateCredential stores a new credential inside the current transaction.
func (q txQueryer) CreateCredential(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
	version int64,
	apiKeyHash string,
	status model.MerchantTerminalCredentialStatus,
	activatedAt time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	return CreateCredential(
		ctx,
		q.Tx,
		merchantTerminalID,
		version,
		apiKeyHash,
		status,
		activatedAt,
		createdAt,
		updatedAt,
	)
}

// MarkRetiring moves a credential to retiring, accepted until retireAt.
func (q txQueryer) MarkRetiring(
	ctx context.Context,
	id model.MerchantTerminalCredentialID,
	retireAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	return MarkRetiring(
		ctx,
		q.Tx,
		id,
		retireAt,
		updatedAt,
	)
}

// MarkRetired moves a credential to retired at retiredAt.
func (q txQueryer) MarkRetired(
	ctx context.Context,
	id model.MerchantTerminalCredentialID,
	retiredAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	return MarkRetired(
		ctx,
		q.Tx,
		id,
		retiredAt,
		updatedAt,
	)
}

// MarkRevoked moves a credential to revoked immediately.
func (q txQueryer) MarkRevoked(
	ctx context.Context,
	id model.MerchantTerminalCredentialID,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	return MarkRevoked(ctx, q.Tx, id, updatedAt)
}

// CreateIssuanceDelivery stores an encrypted issuance delivery for idempotent secret replay.
func (q txQueryer) CreateIssuanceDelivery(
	ctx context.Context,
	merchantTerminalID model.MerchantTerminalID,
	credentialID model.MerchantTerminalCredentialID,
	idempotencyKey string,
	requestFingerprint string,
	apiKeyCiphertext string,
	apiKeyEncryptionKeyID string,
	expiresAt time.Time,
	createdAt time.Time,
) (model.MerchantTerminalIssuanceDelivery, error) {
	return CreateIssuanceDelivery(
		ctx,
		q.Tx,
		merchantTerminalID,
		credentialID,
		idempotencyKey,
		requestFingerprint,
		apiKeyCiphertext,
		apiKeyEncryptionKeyID,
		expiresAt,
		createdAt,
	)
}

// ConsumeIssuanceDelivery records that the secret of a delivery was handed to the caller.
func (q txQueryer) ConsumeIssuanceDelivery(
	ctx context.Context,
	id model.MerchantTerminalIssuanceDeliveryID,
	consumedAt time.Time,
) (model.MerchantTerminalIssuanceDelivery, error) {
	return ConsumeIssuanceDelivery(ctx, q.Tx, id, consumedAt)
}

// ScrubExpiredIssuanceDeliveries scrubs up to limit expired, unpurged deliveries.
func (q txQueryer) ScrubExpiredIssuanceDeliveries(
	ctx context.Context,
	now time.Time,
	limit int,
) (int64, error) {
	return ScrubExpiredIssuanceDeliveries(ctx, q.Tx, now, limit)
}
