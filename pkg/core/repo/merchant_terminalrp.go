package repo

import (
	"context"
	"errors"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var (
	// ErrMerchantTerminalIDTaken indicates that the generated public terminal
	// id is already assigned to another terminal. Callers may retry with a
	// freshly generated id.
	ErrMerchantTerminalIDTaken = errors.New("merchant terminal id already taken")

	// ErrMerchantTerminalExists indicates that the merchant already owns a
	// terminal. Callers may load it to determine whether the request is an
	// idempotent replay.
	ErrMerchantTerminalExists = errors.New("merchant terminal already exists")
)

// MerchantTerminalQueryer defines read-only merchant terminal and credential
// operations.
type MerchantTerminalQueryer interface {
	// GetByTerminalID returns the terminal identified by its public numeric
	// terminal id.
	GetByTerminalID(
		ctx context.Context,
		terminalID model.TerminalID,
	) (model.MerchantTerminal, error)

	// GetByMerchantID returns the terminal owned by merchantID.
	GetByMerchantID(
		ctx context.Context,
		merchantID model.MerchantID,
	) (model.MerchantTerminal, error)

	// GetActiveCredential returns the active credential of a terminal.
	GetActiveCredential(
		ctx context.Context,
		merchantTerminalID model.MerchantTerminalID,
	) (model.MerchantTerminalCredential, error)

	// ListAcceptableCredentials returns the credentials of a terminal that are
	// active, or retiring with a retire deadline after now. It is the lookup
	// used to authenticate a presented API key.
	ListAcceptableCredentials(
		ctx context.Context,
		merchantTerminalID model.MerchantTerminalID,
		now time.Time,
	) ([]model.MerchantTerminalCredential, error)

	// GetCredentialByID returns the credential identified by id.
	GetCredentialByID(
		ctx context.Context,
		id model.MerchantTerminalCredentialID,
	) (model.MerchantTerminalCredential, error)

	// GetIssuanceDeliveryByIdempotencyKey returns the issuance delivery stored
	// for a terminal and idempotency key.
	GetIssuanceDeliveryByIdempotencyKey(
		ctx context.Context,
		merchantTerminalID model.MerchantTerminalID,
		idempotencyKey string,
	) (model.MerchantTerminalIssuanceDelivery, error)

	// ListPermissions returns the server-side permission wire codes assigned to
	// the terminal. The slice may be empty. Callers must FilterKnown before
	// granting access.
	ListPermissions(
		ctx context.Context,
		merchantTerminalID model.MerchantTerminalID,
	) ([]string, error)
}

// MerchantTerminalConnQueryer exposes merchant terminal operations on a
// connection.
type MerchantTerminalConnQueryer interface {
	// MerchantTerminalQueryer provides shared merchant terminal query operations.
	MerchantTerminalQueryer
}

// MerchantTerminalTxQueryer exposes transaction-scoped merchant terminal
// operations.
type MerchantTerminalTxQueryer interface {
	// MerchantTerminalQueryer provides shared merchant terminal query operations.
	MerchantTerminalQueryer

	// LockByTerminalID returns the terminal for terminalID and locks it for
	// state changes.
	LockByTerminalID(
		ctx context.Context,
		terminalID model.TerminalID,
	) (model.MerchantTerminal, error)

	// LockByMerchantID returns the terminal owned by merchantID and locks it
	// for state changes.
	LockByMerchantID(
		ctx context.Context,
		merchantID model.MerchantID,
	) (model.MerchantTerminal, error)

	// CreateTerminal stores a new terminal inside the current transaction.
	CreateTerminal(
		ctx context.Context,
		merchantID model.MerchantID,
		terminalID model.TerminalID,
		status model.MerchantTerminalStatus,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.MerchantTerminal, error)

	// ReplacePermissions replaces all permission assignments for the terminal
	// with permissions (deduplicated). An empty slice clears every grant.
	// Assignments belong to the terminal identity, not a credential version.
	ReplacePermissions(
		ctx context.Context,
		merchantTerminalID model.MerchantTerminalID,
		permissions []string,
		createdAt time.Time,
	) error

	// LockCredentialsForRotation locks and returns every non-final credential
	// (active or retiring) of a terminal, ordered by version ascending, so a
	// rotation can serialize against concurrent rotations and compute the next
	// version.
	LockCredentialsForRotation(
		ctx context.Context,
		merchantTerminalID model.MerchantTerminalID,
	) ([]model.MerchantTerminalCredential, error)

	// CreateCredential stores a new credential inside the current transaction.
	CreateCredential(
		ctx context.Context,
		merchantTerminalID model.MerchantTerminalID,
		version int64,
		apiKeyHash string,
		status model.MerchantTerminalCredentialStatus,
		activatedAt time.Time,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.MerchantTerminalCredential, error)

	// MarkRetiring moves a credential to retiring, accepted until retireAt.
	MarkRetiring(
		ctx context.Context,
		id model.MerchantTerminalCredentialID,
		retireAt time.Time,
		updatedAt time.Time,
	) (model.MerchantTerminalCredential, error)

	// MarkRetired moves an active or retiring credential to retired at
	// retiredAt. An active credential is retired directly by a zero-grace
	// rotation.
	MarkRetired(
		ctx context.Context,
		id model.MerchantTerminalCredentialID,
		retiredAt time.Time,
		updatedAt time.Time,
	) (model.MerchantTerminalCredential, error)

	// MarkRevoked moves a credential to revoked immediately.
	MarkRevoked(
		ctx context.Context,
		id model.MerchantTerminalCredentialID,
		updatedAt time.Time,
	) (model.MerchantTerminalCredential, error)

	// CreateIssuanceDelivery stores an encrypted issuance delivery for
	// idempotent secret replay. requestFingerprint captures the request
	// semantics so a reused idempotencyKey with a different request can be
	// rejected.
	CreateIssuanceDelivery(
		ctx context.Context,
		merchantTerminalID model.MerchantTerminalID,
		credentialID model.MerchantTerminalCredentialID,
		idempotencyKey string,
		requestFingerprint string,
		apiKeyCiphertext string,
		apiKeyEncryptionKeyID string,
		expiresAt time.Time,
		createdAt time.Time,
	) (model.MerchantTerminalIssuanceDelivery, error)

	// ConsumeIssuanceDelivery records that the secret of a delivery was handed
	// to the caller at consumedAt.
	ConsumeIssuanceDelivery(
		ctx context.Context,
		id model.MerchantTerminalIssuanceDeliveryID,
		consumedAt time.Time,
	) (model.MerchantTerminalIssuanceDelivery, error)

	// ScrubExpiredIssuanceDeliveries clears ciphertext and key id and sets
	// purged_at for up to limit deliveries with expires_at <= now that are not
	// yet purged, leaving idempotency tombstones. It returns the number of rows
	// scrubbed.
	ScrubExpiredIssuanceDeliveries(
		ctx context.Context,
		now time.Time,
		limit int,
	) (int64, error)
}

// MerchantTerminal persists merchant terminal identities, API key credentials,
// and issuance deliveries.
type MerchantTerminal interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) MerchantTerminalConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) MerchantTerminalTxQueryer
}
