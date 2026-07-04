package invariants

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// PreflightDiagnostic exposes only our migration preflight's actionable record
// references to an operator CLI. Never use this to serialize an HTTP error.
func PreflightDiagnostic(err error) (string, bool) {
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || !strings.HasPrefix(pg.Message, "Preflight failed:") {
		return "", false
	}

	if (pg.Code == "BN002" && pg.ConstraintName == "installment_has_unsettled_charges") || (pg.Code == "BN004" && pg.ConstraintName == "installment_financial_component_immutable") {
		return pg.Message, true
	}
	return "", false
}
