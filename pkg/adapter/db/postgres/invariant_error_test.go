package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/stretchr/testify/require"
)

func TestManagedInvariantErrorsStaySemanticAndPrivate(t *testing.T) {
	for _, c := range []struct{ code, constraint string }{
		{"BN001", "installment_already_paid"}, {"BN002", "installment_has_unsettled_charges"}, {"BN003", "installment_payment_in_progress"}, {"BN004", "installment_financial_component_immutable"},
	} {
		raw := &pgconn.PgError{Code: c.code, ConstraintName: c.constraint, Message: "private SQL trigger diagnostics"}
		e := MapError(raw)
		require.True(t, cerr.IsCode(e, cerr.CodeConflict))
		var app *cerr.Error
		require.ErrorAs(t, e, &app)
		require.EqualValues(t, "error."+c.constraint, app.MessageKey)
		require.True(t, errors.Is(e, raw))
	}
	require.True(t, cerr.IsCode(MapError(&pgconn.PgError{Code: "BN001", ConstraintName: "unrelated"}), cerr.CodeInternalError))
	require.True(t, cerr.IsCode(MapError(&pgconn.PgError{Code: "P0001", ConstraintName: "installment_already_paid"}), cerr.CodeInternalError))
}
