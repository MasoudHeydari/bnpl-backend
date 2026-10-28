package serdser

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/stretchr/testify/require"
)

func TestManagedInvariantConflictDoesNotExposeDatabaseDiagnostics(t *testing.T) {
	c, w := newBindContext("", "invariant-test")
	SerErr(c, postgres.MapError(&pgconn.PgError{Code: "BN001", ConstraintName: "installment_already_paid", Message: "private SELECT account FROM internal_schema", Where: "guard_installment_charge"}))
	require.Equal(t, http.StatusConflict, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Contains(t, string(w.Body.Bytes()), "already been settled")
	require.NotContains(t, string(w.Body.Bytes()), "internal_schema")
	require.NotContains(t, string(w.Body.Bytes()), "guard_installment_charge")
	require.NotContains(t, string(w.Body.Bytes()), "SELECT")
}
