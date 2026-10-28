package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// MapError converts low-level PostgreSQL/pgx errors into application errors.
//
// Raw PostgreSQL diagnostics are retained only as private causes inside cerr
// and are never public response data. Existing generic classifications are kept
// here for backwards compatibility while repositories migrate incrementally to
// semantic, constraint-specific mapping via helpers such as IsUniqueViolation.
func MapError(err error) error {
	const op = "postgres.MapError"

	if err == nil {
		return nil
	}

	// Preserve already-classified application errors. Pool.Conn/Tx may wrap
	// handler errors before they return here.
	if _, ok := errors.AsType[*cerr.Error](err); ok {
		return err
	}

	// No rows returned is a standard "not found" case in repositories.
	if errors.Is(err, pgx.ErrNoRows) {
		return cerr.ResourceNotFound(err)
	}

	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return mapPgError(pgErr)
	}

	if _, ok := errors.AsType[*pgconn.ConnectError](err); ok {
		return cerr.ServiceUnavailable(fmt.Errorf("%s: failed to connect to database", op))
	}

	// Any unknown error is treated as internal failure.
	return cerr.InternalError(err)
}

// IsUniqueViolation reports whether err contains a PostgreSQL unique-constraint
// violation. When constraint is non-empty, it must match PgError.ConstraintName.
//
// Repositories should use this helper to recognize constraints they own and map
// them to semantic application errors without exposing PostgreSQL names upward.
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != pgerrcode.UniqueViolation {
		return false
	}

	return constraint == "" || pgErr.ConstraintName == constraint
}

// mapPgError provides the transitional generic PostgreSQL classification.
//
// Constraint names/columns are intentionally not copied into public metadata.
// Known business constraints should be remapped by the owning repository.
func mapPgError(err *pgconn.PgError) error {
	keys := map[string]struct{ constraint, key string }{
		"BN001": {"installment_already_paid", "error.installment_already_paid"},
		"BN002": {"installment_has_unsettled_charges", "error.installment_has_unsettled_charges"},
		"BN003": {"installment_payment_in_progress", "error.installment_payment_in_progress"},
		"BN004": {"installment_financial_component_immutable", "error.installment_financial_component_immutable"},
	}
	if managed, ok := keys[err.Code]; ok && err.ConstraintName == managed.constraint {
		return cerr.NewPublic(cerr.CodeConflict, cerr.MessageKey(managed.key), err, cerr.Details{Violations: []cerr.Violation{{Code: managed.constraint, MessageKey: cerr.MessageKey(managed.key)}}})
	}
	switch err.Code {

	// Unique constraint violations typically indicate duplicate business keys
	// (e.g. duplicate email, duplicate contract, idempotency conflicts).
	case pgerrcode.UniqueViolation:
		return cerr.ValidationFailed(err)

	case pgerrcode.ForeignKeyViolation,
		pgerrcode.NotNullViolation,
		pgerrcode.CheckViolation,
		pgerrcode.InvalidTextRepresentation,
		pgerrcode.NumericValueOutOfRange:
		return cerr.InvalidArgument(err)

	// Connection-level issues (DB down, network issues, timeout, etc.).
	case pgerrcode.ConnectionException:
		return cerr.ServiceUnavailable(err)

	// Deadlock and serialization failures are, by definition, the database
	// aborting one side of a concurrent conflict so the other can proceed.
	// They are never a sign of invalid data or an internal bug, so they map to
	// the same client-facing conflict semantics as a unique violation rather
	// than leaking as an internal error.
	case pgerrcode.DeadlockDetected, pgerrcode.SerializationFailure:
		return cerr.Conflict(err)

	default:
		// Any unknown PostgreSQL error is treated as internal to avoid leaking DB semantics.
		return cerr.InternalError(err)
	}
}
