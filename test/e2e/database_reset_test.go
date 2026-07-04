//go:build e2e

package e2e_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// TestE2E_DatabaseReset_PreservesMigrationMetadata proves resetDatabase wipes
// business data without touching bnpl.migrations. A prior version of this
// helper discovered every table in the bnpl schema at runtime and truncated
// all of them, including migration metadata; resetDatabase now truncates only
// the explicit mutableE2ETables allowlist.
func TestE2E_DatabaseReset_PreservesMigrationMetadata(t *testing.T) {
	resetDatabase(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	marker := "zzz_e2e_reset_guard_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	checksum := "sha256:" + strings.Repeat("a", 64)

	_, err := testDB.Exec(ctx, `
		INSERT INTO bnpl.migrations (name, checksum, created_at)
		VALUES ($1, $2, now())
	`, marker, checksum)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = testDB.Exec(cleanupCtx, `DELETE FROM bnpl.migrations WHERE name = $1`, marker)
	})

	resetDatabase(t)

	var count int
	require.NoError(t, testDB.QueryRow(
		ctx,
		`SELECT count(*) FROM bnpl.migrations WHERE name = $1`,
		marker,
	).Scan(&count))
	assert.Equal(t, 1, count, "resetDatabase must not remove rows from bnpl.migrations")
}

// TestE2E_DatabaseReset_RemovesBusinessData proves resetDatabase actually
// wipes mutable business data, using a table (customer_profiles) with no
// public HTTP creation path so the row can only exist here from direct setup.
func TestE2E_DatabaseReset_RemovesBusinessData(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	insertCustomerProfile(t, externalCustomerID, model.ActiveCustomerStatus)

	_, found := queryCustomerProfile(t, externalCustomerID)
	require.True(t, found, "test setup must have persisted the customer profile row")

	resetDatabase(t)

	_, found = queryCustomerProfile(t, externalCustomerID)
	assert.False(t, found, "resetDatabase must remove mutable business data such as customer_profiles rows")
}

// -----------------------------------------------------------------------------
// Reset table policy drift detection (checkResetTablePolicy)
// -----------------------------------------------------------------------------
//
// These are pure unit tests of the policy-comparison logic: they never touch
// testDB or the bnpl schema, so they can prove the drift/duplicate/overlap
// detection works without needing a real migration to add or remove a table.

// TestResetTablePolicy_AcceptsCurrentConfiguration proves the live
// mutableE2ETables/preservedE2ETables lists are internally consistent (no
// duplicates, no overlap) when checked against exactly the tables they
// themselves declare.
func TestResetTablePolicy_AcceptsCurrentConfiguration(t *testing.T) {
	actualTables := make([]string, 0, len(mutableE2ETables)+len(preservedE2ETables))
	actualTables = append(actualTables, mutableE2ETables...)
	actualTables = append(actualTables, preservedE2ETables...)

	require.NoError(t, checkResetTablePolicy(actualTables, mutableE2ETables, preservedE2ETables))
	assert.Contains(t, preservedE2ETables, "migrations", "migration metadata must always be configured as preserved")
}

// TestResetTablePolicy_DetectsUnclassifiedTable proves that a table present
// in the schema but missing from both mutableE2ETables and
// preservedE2ETables (for example, a new table added by a migration without
// updating the reset policy) is rejected with a clear diagnostic, instead of
// silently leaking data between tests.
func TestResetTablePolicy_DetectsUnclassifiedTable(t *testing.T) {
	actualTables := append(append([]string{}, mutableE2ETables...), preservedE2ETables...)
	actualTables = append(actualTables, "some_new_table_added_by_a_migration")

	err := checkResetTablePolicy(actualTables, mutableE2ETables, preservedE2ETables)
	require.Error(t, err)

	violations, ok := err.(resetTablePolicyViolations)
	require.True(t, ok, "expected a resetTablePolicyViolations error, got %T", err)
	assert.Contains(t, violations.Unclassified, "some_new_table_added_by_a_migration")
	assert.Contains(t, err.Error(), "some_new_table_added_by_a_migration")
}

// TestResetTablePolicy_DetectsMissingConfiguredTable proves that a table
// configured in the reset policy but absent from the live schema (for
// example, after a table rename) is rejected instead of silently being a
// no-op TRUNCATE target.
func TestResetTablePolicy_DetectsMissingConfiguredTable(t *testing.T) {
	mutable := []string{"customer_profiles", "a_table_that_was_renamed_or_dropped"}
	preserved := []string{"migrations"}
	actualTables := []string{"customer_profiles", "migrations"}

	err := checkResetTablePolicy(actualTables, mutable, preserved)
	require.Error(t, err)

	violations, ok := err.(resetTablePolicyViolations)
	require.True(t, ok, "expected a resetTablePolicyViolations error, got %T", err)
	assert.Contains(t, violations.MissingFromSchema, "a_table_that_was_renamed_or_dropped")
}

// TestResetTablePolicy_RejectsDuplicateAndOverlappingEntries proves that
// duplicate entries within a single list, and a table configured in both
// mutableE2ETables and preservedE2ETables, are both rejected rather than
// silently tolerated.
func TestResetTablePolicy_RejectsDuplicateAndOverlappingEntries(t *testing.T) {
	mutable := []string{"customer_profiles", "customer_profiles", "decisions"}
	preserved := []string{"migrations", "migrations", "decisions"}
	actualTables := []string{"customer_profiles", "decisions", "migrations"}

	err := checkResetTablePolicy(actualTables, mutable, preserved)
	require.Error(t, err)

	violations, ok := err.(resetTablePolicyViolations)
	require.True(t, ok, "expected a resetTablePolicyViolations error, got %T", err)
	assert.Contains(t, violations.DuplicateMutable, "customer_profiles")
	assert.Contains(t, violations.DuplicatePreserved, "migrations")
	assert.Contains(t, violations.OverlappingBoth, "decisions")
}

// TestResetTablePolicy_ValidConfiguration_NoViolations is a control case
// proving checkResetTablePolicy reports no error when the schema and policy
// agree exactly, so the failure tests above are actually exercising drift
// detection rather than an always-failing comparison.
func TestResetTablePolicy_ValidConfiguration_NoViolations(t *testing.T) {
	mutable := []string{"customer_profiles", "decisions"}
	preserved := []string{"migrations"}
	actualTables := []string{"customer_profiles", "decisions", "migrations"}

	assert.NoError(t, checkResetTablePolicy(actualTables, mutable, preserved))
}
