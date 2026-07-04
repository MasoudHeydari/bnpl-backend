package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

func TestMapError_Nil(t *testing.T) {
	t.Parallel()
	require.NoError(t, MapError(nil))
}

func TestMapError_NoRows(t *testing.T) {
	t.Parallel()

	err := MapError(pgx.ErrNoRows)
	require.True(t, cerr.IsCode(err, cerr.CodeResourceNotFound))
}

func TestMapError_PgCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
		want cerr.Code
	}{
		{"UniqueViolation", pgerrcode.UniqueViolation, cerr.CodeValidationFailed},
		{"ForeignKeyViolation", pgerrcode.ForeignKeyViolation, cerr.CodeInvalidArgument},
		{"NotNullViolation", pgerrcode.NotNullViolation, cerr.CodeInvalidArgument},
		{"CheckViolation", pgerrcode.CheckViolation, cerr.CodeInvalidArgument},
		{"InvalidTextRepresentation", pgerrcode.InvalidTextRepresentation, cerr.CodeInvalidArgument},
		{"NumericValueOutOfRange", pgerrcode.NumericValueOutOfRange, cerr.CodeInvalidArgument},
		{"ConnectionException", pgerrcode.ConnectionException, cerr.CodeServiceUnavailable},
		{"DeadlockDetected", pgerrcode.DeadlockDetected, cerr.CodeConflict},
		{"SerializationFailure", pgerrcode.SerializationFailure, cerr.CodeConflict},
		{"Unknown", "99999", cerr.CodeInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := MapError(&pgconn.PgError{Code: tt.code})
			require.True(t, cerr.IsCode(err, tt.want))
		})
	}
}

func TestIsUniqueViolation(t *testing.T) {
	t.Parallel()

	const constraint = "merchant_profiles_merchant_id_key"
	pgErr := &pgconn.PgError{
		Code:           pgerrcode.UniqueViolation,
		ConstraintName: constraint,
	}

	mapped := MapError(pgErr)
	require.True(t, cerr.IsCode(mapped, cerr.CodeValidationFailed))
	require.True(t, IsUniqueViolation(mapped, constraint))
	require.True(t, IsUniqueViolation(mapped, ""))
	require.False(t, IsUniqueViolation(mapped, "other_constraint"))
}

func TestMapError_UnknownError(t *testing.T) {
	t.Parallel()

	err := MapError(errors.New("driver failure"))
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
}

func TestMapError_PreservesClassifiedApplicationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want cerr.Code
	}{
		{
			name: "IdempotencyConflict",
			err:  cerr.IdempotencyConflict(errors.New("idempotency key reused with different checkout inputs")),
			want: cerr.CodeIdempotencyConflict,
		},
		{
			name: "WrappedIdempotencyConflictFromTxHandler",
			err: fmt.Errorf(
				"handler: %w",
				cerr.IdempotencyConflict(errors.New("idempotency key reused with different checkout inputs")),
			),
			want: cerr.CodeIdempotencyConflict,
		},
		{
			name: "ResourceNotFound",
			err:  cerr.ResourceNotFound(errors.New("missing")),
			want: cerr.CodeResourceNotFound,
		},
		{
			name: "Conflict",
			err:  cerr.Conflict(errors.New("state conflict")),
			want: cerr.CodeConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := MapError(tt.err)
			require.True(t, cerr.IsCode(err, tt.want), "got %v", err)

			var ce *cerr.Error
			require.True(t, errors.As(err, &ce))
			require.NotEqual(t, cerr.CodeInternalError, ce.Code)
		})
	}
}
