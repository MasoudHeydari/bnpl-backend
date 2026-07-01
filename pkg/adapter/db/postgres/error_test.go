package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/stretchr/testify/require"
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
		{"ConnectionException", pgerrcode.ConnectionException, cerr.CodeInternalError},
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

func TestMapError_UnknownError(t *testing.T) {
	t.Parallel()

	err := MapError(errors.New("driver failure"))
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
}
