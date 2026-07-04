package invariants

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/stretchr/testify/require"
)

func TestPreflightDiagnosticIsOperatorOnlyAndExact(t *testing.T) {
	raw := &pgconn.PgError{Code: "BN002", ConstraintName: "installment_has_unsettled_charges", Message: "Preflight failed: customer=c contract=k installment=i charge=x. Reconcile explicitly."}
	msg, ok := PreflightDiagnostic(postgres.MapError(raw))
	require.True(t, ok)
	require.Contains(t, msg, "installment=i")
	raw.ConstraintName = "other"
	_, ok = PreflightDiagnostic(raw)
	require.False(t, ok)
	raw.ConstraintName = "installment_has_unsettled_charges"
	raw.Message = "private runtime SQL diagnostics"
	_, ok = PreflightDiagnostic(raw)
	require.False(t, ok)
}
