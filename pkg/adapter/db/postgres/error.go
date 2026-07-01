package postgres

import (
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// MapError converts low-level PostgreSQL/pgx errors into domain-safe cerr errors.
//
// Responsibility of this layer:
//   - Hide database-specific error details
//   - Convert SQL/driver errors into stable application errors
//   - Ensure API layer never leaks raw DB errors
//
// This function is used by all postgres repositories.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	// No rows returned is a standard "not found" case in repositories.
	if errors.Is(err, pgx.ErrNoRows) {
		return cerr.ResourceNotFound(err)
	}

	// Go 1.26+: type-safe error extraction using AsType.
	// This replaces unsafe manual casting patterns.
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return mapPgError(pgErr)
	}

	// Any unknown error is treated as internal failure.
	return cerr.InternalError(err)
}

// mapPgError translates PostgreSQL-specific error codes into cerr errors.
//
// This ensures:
//   - DB constraint errors become client-safe validation errors
//   - Infrastructure failures remain internal
//   - API layer receives consistent error semantics
func mapPgError(err *pgconn.PgError) error {
	switch err.Code {

	// Unique constraint violations typically indicate duplicate business keys
	// (e.g. duplicate email, duplicate contract, idempotency conflicts).
	case pgerrcode.UniqueViolation:
		return cerr.ValidationFailed(
			err,
			map[string]any{
				"column":     err.ColumnName,
				"constraint": err.ConstraintName,
			},
		)

	// Foreign key violations indicate invalid references between aggregates.
	case pgerrcode.ForeignKeyViolation:
		return cerr.InvalidArgument(err, map[string]any{
			"constraint": err.ConstraintName,
		})

	// These are client input validation problems at SQL level.
	case pgerrcode.NotNullViolation,
		pgerrcode.CheckViolation,
		pgerrcode.InvalidTextRepresentation,
		pgerrcode.NumericValueOutOfRange:
		return cerr.InvalidArgument(err, map[string]any{
			"column":     err.ColumnName,
			"constraint": err.ConstraintName,
		})

	// Connection-level issues (DB down, network issues, timeout, etc.)
	case pgerrcode.ConnectionException:
		return cerr.InternalError(err)

	default:
		// Any unknown PostgreSQL error is treated as internal to avoid leaking DB semantics.
		return cerr.InternalError(err)
	}
}
