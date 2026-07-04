package migrationsrp_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
)

// TestSchema_ReturnDestinationConstraintsDocumentsGrammar is a schema-source
// documentation check. It does not exercise PostgreSQL. Real enforcement is
// covered by the integration-tagged tests in
// return_destination_constraint_integration_test.go.
func TestSchema_ReturnDestinationConstraintsDocumentsGrammar(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	schemaPath := filepath.Join(filepath.Dir(thisFile), "000_schema.sql")
	raw, err := os.ReadFile(schemaPath)
	require.NoError(t, err)

	schema := string(raw)
	require.Contains(t, schema, "chk_merchant_return_destinations_destination_name_format")
	require.Contains(t, schema, "chk_checkout_sessions_return_destination_format")
	require.Contains(t, schema, "destination_name ~ '"+returndestination.DestinationNamePattern+"'")
	require.Contains(t, schema, "return_destination ~ '"+returndestination.DestinationNamePattern+"'")
	require.Contains(t, schema, "char_length(destination_name) BETWEEN 1 AND 64")
	require.Contains(t, schema, "char_length(return_destination) BETWEEN 1 AND 64")
}
