package merchantterminalrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	terminalMerchantIDUnique = "uq_merchant_terminals_merchant_id"
	terminalTerminalIDUnique = "uq_merchant_terminals_terminal_id"
)

// merchantTerminalColumns is the shared projection of terminal reads.
const merchantTerminalColumns = `
	id,
	merchant_id,
	terminal_id,
	status,
	created_at,
	updated_at
`

// credentialColumns is the shared projection of credential reads.
const credentialColumns = `
	id,
	merchant_terminal_id,
	version,
	api_key_hash,
	status,
	created_at,
	activated_at,
	retire_at,
	retired_at,
	updated_at
`

// issuanceDeliveryColumns is the shared projection of issuance delivery reads.
// Secret columns are NULL on purged tombstones; COALESCE maps them to empty
// strings so they scan into the model's string fields.
const issuanceDeliveryColumns = `
	id,
	merchant_terminal_id,
	credential_id,
	idempotency_key,
	request_fingerprint,
	COALESCE(api_key_ciphertext, '') AS api_key_ciphertext,
	COALESCE(api_key_encryption_key_id, '') AS api_key_encryption_key_id,
	expires_at,
	consumed_at,
	purged_at,
	created_at
`

// GetByTerminalID returns a terminal by its public terminal id.
func GetByTerminalID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	terminalID model.TerminalID,
) (model.MerchantTerminal, error) {
	const query = `SELECT` + merchantTerminalColumns + `FROM merchant_terminals WHERE terminal_id = $1`

	return postgres.QueryOne[model.MerchantTerminal](
		ctx,
		q,
		"get merchant terminal by terminal id",
		query,
		terminalID,
	)
}

// GetByMerchantID returns the terminal owned by merchantID.
func GetByMerchantID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
) (model.MerchantTerminal, error) {
	const query = `SELECT` + merchantTerminalColumns + `FROM merchant_terminals WHERE merchant_id = $1`

	return postgres.QueryOne[model.MerchantTerminal](
		ctx,
		q,
		"get merchant terminal by merchant id",
		query,
		merchantID,
	)
}

// LockByTerminalID returns a terminal by its public terminal id and locks it for update.
func LockByTerminalID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	terminalID model.TerminalID,
) (model.MerchantTerminal, error) {
	const query = `SELECT` + merchantTerminalColumns + `FROM merchant_terminals WHERE terminal_id = $1 FOR UPDATE`

	return postgres.QueryOne[model.MerchantTerminal](
		ctx,
		q,
		"lock merchant terminal by terminal id",
		query,
		terminalID,
	)
}

// LockByMerchantID returns the terminal owned by merchantID and locks it for update.
func LockByMerchantID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
) (model.MerchantTerminal, error) {
	const query = `SELECT` + merchantTerminalColumns + `FROM merchant_terminals WHERE merchant_id = $1 FOR UPDATE`

	return postgres.QueryOne[model.MerchantTerminal](
		ctx,
		q,
		"lock merchant terminal by merchant id",
		query,
		merchantID,
	)
}

// CreateTerminal inserts a terminal and returns the persisted row.
//
// A duplicate public terminal id maps to repo.ErrMerchantTerminalIDTaken and a
// second terminal for the same merchant maps to repo.ErrMerchantTerminalExists.
func CreateTerminal[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
	terminalID model.TerminalID,
	status model.MerchantTerminalStatus,
	createdAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminal, error) {
	const op = "merchantterminalrp.CreateTerminal"

	err := postgres.RequiredTimes(
		map[string]time.Time{
			"created_at": createdAt,
			"updated_at": updatedAt,
		},
	)
	if err != nil {
		return model.MerchantTerminal{}, err
	}

	const query = `
INSERT INTO merchant_terminals (
	merchant_id,
	terminal_id,
	status,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5)
RETURNING` + merchantTerminalColumns

	terminal, err := postgres.QueryOne[model.MerchantTerminal](
		ctx,
		q,
		"create merchant terminal",
		query,
		merchantID,
		terminalID,
		status,
		createdAt,
		updatedAt,
	)
	if err != nil {
		if postgres.IsUniqueViolation(err, terminalTerminalIDUnique) {
			return model.MerchantTerminal{}, cerr.Conflict(
				fmt.Errorf("%s: %w", op, repo.ErrMerchantTerminalIDTaken),
			)
		}

		if postgres.IsUniqueViolation(err, terminalMerchantIDUnique) {
			return model.MerchantTerminal{}, cerr.Conflict(
				fmt.Errorf("%s: %w", op, repo.ErrMerchantTerminalExists),
			)
		}

		return model.MerchantTerminal{}, err
	}

	return terminal, nil
}

// GetActiveCredential returns the active credential of a terminal.
func GetActiveCredential[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantTerminalID model.MerchantTerminalID,
) (model.MerchantTerminalCredential, error) {
	const query = `SELECT` + credentialColumns + `
FROM merchant_terminal_credentials
WHERE merchant_terminal_id = $1 AND status = 'active'`

	return postgres.QueryOne[model.MerchantTerminalCredential](
		ctx,
		q,
		"get active merchant terminal credential",
		query,
		merchantTerminalID,
	)
}

// ListAcceptableCredentials returns the active credentials and the retiring
// credentials whose retire deadline is after now, newest version first.
func ListAcceptableCredentials[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantTerminalID model.MerchantTerminalID,
	now time.Time,
) ([]model.MerchantTerminalCredential, error) {
	err := postgres.RequiredTime("now", now)
	if err != nil {
		return nil, err
	}

	const query = `SELECT` + credentialColumns + `
FROM merchant_terminal_credentials
WHERE merchant_terminal_id = $1
	AND (
		status = 'active'
		OR (status = 'retiring' AND retire_at > $2)
	)
ORDER BY version DESC`

	return postgres.QueryAll[model.MerchantTerminalCredential](
		ctx,
		q,
		"list acceptable merchant terminal credentials",
		query,
		merchantTerminalID,
		now,
	)
}

// GetCredentialByID returns a credential by id.
func GetCredentialByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.MerchantTerminalCredentialID,
) (model.MerchantTerminalCredential, error) {
	const query = `SELECT` + credentialColumns + `FROM merchant_terminal_credentials WHERE id = $1`

	return postgres.QueryOne[model.MerchantTerminalCredential](
		ctx,
		q,
		"get merchant terminal credential by id",
		query,
		id,
	)
}

// LockCredentialsForRotation locks the active and retiring credentials of a
// terminal ordered by version ascending.
func LockCredentialsForRotation[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantTerminalID model.MerchantTerminalID,
) ([]model.MerchantTerminalCredential, error) {
	const query = `SELECT` + credentialColumns + `
FROM merchant_terminal_credentials
WHERE merchant_terminal_id = $1 AND status IN ('active', 'retiring')
ORDER BY version ASC
FOR UPDATE`

	return postgres.QueryAll[model.MerchantTerminalCredential](
		ctx,
		q,
		"lock merchant terminal credentials for rotation",
		query,
		merchantTerminalID,
	)
}

// CreateCredential inserts a credential and returns the persisted row.
func CreateCredential[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantTerminalID model.MerchantTerminalID,
	version int64,
	apiKeyHash string,
	status model.MerchantTerminalCredentialStatus,
	activatedAt time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	err := postgres.RequiredTimes(
		map[string]time.Time{
			"activated_at": activatedAt,
			"created_at":   createdAt,
			"updated_at":   updatedAt,
		},
	)
	if err != nil {
		return model.MerchantTerminalCredential{}, err
	}

	const query = `
INSERT INTO merchant_terminal_credentials (
	merchant_terminal_id,
	version,
	api_key_hash,
	status,
	activated_at,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING` + credentialColumns

	return postgres.QueryOne[model.MerchantTerminalCredential](
		ctx,
		q,
		"create merchant terminal credential",
		query,
		merchantTerminalID,
		version,
		apiKeyHash,
		status,
		activatedAt,
		createdAt,
		updatedAt,
	)
}

// MarkRetiring moves an active credential to retiring. It returns a not-found
// error when no active credential has the id.
func MarkRetiring[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.MerchantTerminalCredentialID,
	retireAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	err := postgres.RequiredTimes(
		map[string]time.Time{
			"retire_at":  retireAt,
			"updated_at": updatedAt,
		},
	)
	if err != nil {
		return model.MerchantTerminalCredential{}, err
	}

	const query = `
UPDATE merchant_terminal_credentials
SET status = 'retiring', retire_at = $2, updated_at = $3
WHERE id = $1 AND status = 'active'
RETURNING` + credentialColumns

	return postgres.QueryOne[model.MerchantTerminalCredential](
		ctx,
		q,
		"mark merchant terminal credential retiring",
		query,
		id,
		retireAt,
		updatedAt,
	)
}

// MarkRetired moves an active or retiring credential to retired. An active
// credential is retired directly when a rotation uses a zero grace period. It
// returns a not-found error when no such credential has the id.
func MarkRetired[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.MerchantTerminalCredentialID,
	retiredAt time.Time,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	err := postgres.RequiredTimes(
		map[string]time.Time{
			"retired_at": retiredAt,
			"updated_at": updatedAt,
		},
	)
	if err != nil {
		return model.MerchantTerminalCredential{}, err
	}

	const query = `
UPDATE merchant_terminal_credentials
SET status = 'retired', retired_at = $2, updated_at = $3
WHERE id = $1 AND status IN ('active', 'retiring')
RETURNING` + credentialColumns

	return postgres.QueryOne[model.MerchantTerminalCredential](
		ctx,
		q,
		"mark merchant terminal credential retired",
		query,
		id,
		retiredAt,
		updatedAt,
	)
}

// MarkRevoked moves an active or retiring credential to revoked. It returns a
// not-found error when no such credential has the id.
func MarkRevoked[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.MerchantTerminalCredentialID,
	updatedAt time.Time,
) (model.MerchantTerminalCredential, error) {
	err := postgres.RequiredTime("updated_at", updatedAt)
	if err != nil {
		return model.MerchantTerminalCredential{}, err
	}

	const query = `
UPDATE merchant_terminal_credentials
SET status = 'revoked', updated_at = $2
WHERE id = $1 AND status IN ('active', 'retiring')
RETURNING` + credentialColumns

	return postgres.QueryOne[model.MerchantTerminalCredential](
		ctx,
		q,
		"mark merchant terminal credential revoked",
		query,
		id,
		updatedAt,
	)
}

// GetIssuanceDeliveryByIdempotencyKey returns the delivery stored for a
// terminal and idempotency key.
func GetIssuanceDeliveryByIdempotencyKey[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantTerminalID model.MerchantTerminalID,
	idempotencyKey string,
) (model.MerchantTerminalIssuanceDelivery, error) {
	const query = `SELECT` + issuanceDeliveryColumns + `
FROM merchant_terminal_issuance_deliveries
WHERE merchant_terminal_id = $1 AND idempotency_key = $2`

	return postgres.QueryOne[model.MerchantTerminalIssuanceDelivery](
		ctx,
		q,
		"get merchant terminal issuance delivery by idempotency key",
		query,
		merchantTerminalID,
		idempotencyKey,
	)
}

// CreateIssuanceDelivery inserts an encrypted issuance delivery and returns the persisted row.
func CreateIssuanceDelivery[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantTerminalID model.MerchantTerminalID,
	credentialID model.MerchantTerminalCredentialID,
	idempotencyKey string,
	requestFingerprint string,
	apiKeyCiphertext string,
	apiKeyEncryptionKeyID string,
	expiresAt time.Time,
	createdAt time.Time,
) (model.MerchantTerminalIssuanceDelivery, error) {
	err := postgres.RequiredTimes(
		map[string]time.Time{
			"expires_at": expiresAt,
			"created_at": createdAt,
		},
	)
	if err != nil {
		return model.MerchantTerminalIssuanceDelivery{}, err
	}

	const query = `
INSERT INTO merchant_terminal_issuance_deliveries (
	merchant_terminal_id,
	credential_id,
	idempotency_key,
	request_fingerprint,
	api_key_ciphertext,
	api_key_encryption_key_id,
	expires_at,
	created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING` + issuanceDeliveryColumns

	return postgres.QueryOne[model.MerchantTerminalIssuanceDelivery](
		ctx,
		q,
		"create merchant terminal issuance delivery",
		query,
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

// ConsumeIssuanceDelivery records the first time the secret of a delivery was
// handed to the caller. Later calls keep the original timestamp.
func ConsumeIssuanceDelivery[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.MerchantTerminalIssuanceDeliveryID,
	consumedAt time.Time,
) (model.MerchantTerminalIssuanceDelivery, error) {
	err := postgres.RequiredTime("consumed_at", consumedAt)
	if err != nil {
		return model.MerchantTerminalIssuanceDelivery{}, err
	}

	const query = `
UPDATE merchant_terminal_issuance_deliveries
SET consumed_at = COALESCE(consumed_at, $2)
WHERE id = $1
RETURNING` + issuanceDeliveryColumns

	return postgres.QueryOne[model.MerchantTerminalIssuanceDelivery](
		ctx,
		q,
		"consume merchant terminal issuance delivery",
		query,
		id,
		consumedAt,
	)
}

// ListPermissions returns the server-side permission wire codes for a terminal.
func ListPermissions[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantTerminalID model.MerchantTerminalID,
) ([]string, error) {
	const query = `
SELECT permission
FROM merchant_terminal_permissions
WHERE merchant_terminal_id = $1
ORDER BY permission`

	type permissionRow struct {
		Permission string `db:"permission"`
	}

	rows, err := postgres.QueryAll[permissionRow](
		ctx,
		q,
		"list merchant terminal permissions",
		query,
		merchantTerminalID,
	)
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Permission)
	}

	return out, nil
}

// ReplacePermissions replaces every permission assignment for the terminal.
// An empty permissions slice clears all grants. Duplicates are ignored.
func ReplacePermissions[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantTerminalID model.MerchantTerminalID,
	permissions []string,
	createdAt time.Time,
) error {
	const op = "merchantterminalrp.ReplacePermissions"

	err := postgres.RequiredTime("created_at", createdAt)
	if err != nil {
		return err
	}

	const deleteQuery = `
DELETE FROM merchant_terminal_permissions
WHERE merchant_terminal_id = $1`

	_, err = q.Exec(ctx, deleteQuery, merchantTerminalID)
	if err != nil {
		return fmt.Errorf("%s: clear permissions: %w", op, postgres.MapError(err))
	}

	seen := make(map[string]struct{}, len(permissions))

	const insertQuery = `
INSERT INTO merchant_terminal_permissions (
	merchant_terminal_id,
	permission,
	created_at
) VALUES ($1, $2, $3)`

	for _, permission := range permissions {
		if permission == "" {
			return fmt.Errorf("%s: permission must not be empty", op)
		}

		if _, ok := seen[permission]; ok {
			continue
		}

		seen[permission] = struct{}{}

		_, err = q.Exec(
			ctx,
			insertQuery,
			merchantTerminalID,
			permission,
			createdAt,
		)
		if err != nil {
			return fmt.Errorf("%s: insert permission: %w", op, postgres.MapError(err))
		}
	}

	return nil
}

// ScrubExpiredIssuanceDeliveries clears the secret columns of up to limit
// expired, still unpurged deliveries and stamps purged_at, turning them into
// idempotency tombstones. It returns the number of rows scrubbed.
//
// Rows are claimed with FOR UPDATE SKIP LOCKED so concurrent workers and
// in-flight replays never block each other.
func ScrubExpiredIssuanceDeliveries[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	limit int,
) (int64, error) {
	const op = "merchantterminalrp.ScrubExpiredIssuanceDeliveries"

	err := postgres.RequiredTime("now", now)
	if err != nil {
		return 0, err
	}

	if limit <= 0 {
		return 0, fmt.Errorf("%s: limit must be positive", op)
	}

	const query = `
UPDATE merchant_terminal_issuance_deliveries
SET api_key_ciphertext = NULL,
	api_key_encryption_key_id = NULL,
	purged_at = $1
WHERE id IN (
	SELECT id
	FROM merchant_terminal_issuance_deliveries
	WHERE expires_at <= $1 AND purged_at IS NULL
	ORDER BY expires_at
	LIMIT $2
	FOR UPDATE SKIP LOCKED
)`

	affected, err := q.Exec(ctx, query, now, limit)
	if err != nil {
		return 0, fmt.Errorf("%s: scrub expired issuance deliveries: %w", op, postgres.MapError(err))
	}

	return affected, nil
}
