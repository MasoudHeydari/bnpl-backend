package creditaccountrp_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

func TestCreditTransactionBusinessFactUniqueViolation_IsRecognized(t *testing.T) {
	t.Parallel()

	pgErr := &pgconn.PgError{
		Code:           pgerrcode.UniqueViolation,
		ConstraintName: "uq_credit_transactions_business_fact",
	}
	mapped := postgres.MapError(pgErr)
	wrapped := errors.New("append credit transaction: " + mapped.Error())
	_ = wrapped

	require.True(t, postgres.IsUniqueViolation(mapped, "uq_credit_transactions_business_fact"))
	require.True(t, cerr.IsCode(mapped, cerr.CodeValidationFailed))
}
