//go:build e2e || legacy_e2e

package e2e_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// decisionDTO mirrors the JSON Decision contract shared by credit-limit and
// checkout decision responses.
type decisionDTO struct {
	ID            string   `json:"id"`
	Type          string   `json:"type"`
	TargetType    string   `json:"target_type"`
	TargetID      string   `json:"target_id"`
	Outcome       string   `json:"outcome"`
	ReasonCodes   []string `json:"reason_codes"`
	PolicyID      string   `json:"policy_id"`
	PolicyVersion string   `json:"policy_version"`
}

type customerProfileRow struct {
	ID                uuid.UUID
	IdentitySubjectID model.IdentitySubjectID
	Jurisdiction      string
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func queryCustomerProfile(t *testing.T, identitySubjectID model.IdentitySubjectID) (customerProfileRow, bool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var idStr, identitySubjectIDStr string
	var r customerProfileRow

	err := testDB.QueryRow(ctx, `
		SELECT id, identity_subject_id, jurisdiction, status, created_at, updated_at
		FROM bnpl.customer_profiles
		WHERE identity_subject_id = $1
	`, identitySubjectID).Scan(
		&idStr, &identitySubjectIDStr,
		&r.Jurisdiction, &r.Status, &r.CreatedAt, &r.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return customerProfileRow{}, false
	}

	require.NoError(t, err)

	r.ID = assertValidUUID(t, idStr)
	r.IdentitySubjectID = model.IdentitySubjectID(assertValidUUID(t, identitySubjectIDStr))

	return r, true
}

// insertCustomerProfile creates a customer profile row directly. Initial credit
// assignment authenticates with x-auth-subject-id set to the profile's
// IdentitySubjectID (customer_profiles.identity_subject_id). The returned
// CustomerID is the internal BNPL primary key (customer_profiles.id).
func insertCustomerProfile(t *testing.T, identitySubjectID model.IdentitySubjectID, status model.CustomerStatus) model.CustomerID {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var idStr string
	require.NoError(t, testDB.QueryRow(ctx, `
		INSERT INTO bnpl.customer_profiles (identity_subject_id, jurisdiction, status, created_at, updated_at)
		VALUES ($1, 'IR', $2, now(), now())
		RETURNING id
	`, identitySubjectID, string(status)).Scan(&idStr))

	return model.CustomerID(assertValidUUID(t, idStr))
}

func countAllCustomerProfiles(t *testing.T) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	require.NoError(t, testDB.QueryRow(ctx, `SELECT count(*) FROM bnpl.customer_profiles`).Scan(&count))

	return count
}

type decisionRow struct {
	ID            uuid.UUID
	Type          string
	TargetType    string
	TargetID      uuid.UUID
	Outcome       string
	ReasonCodes   []string
	PolicyID      string
	PolicyVersion string
}

func queryDecisionByID(t *testing.T, id model.DecisionID) decisionRow {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var idStr, targetIDStr string
	var r decisionRow

	require.NoError(t, testDB.QueryRow(ctx, `
		SELECT id, type, target_type, target_id, outcome, reason_codes, policy_id, policy_version
		FROM bnpl.decisions
		WHERE id = $1
	`, id).Scan(&idStr, &r.Type, &r.TargetType, &targetIDStr, &r.Outcome, &r.ReasonCodes, &r.PolicyID, &r.PolicyVersion))

	r.ID = assertValidUUID(t, idStr)
	r.TargetID = assertValidUUID(t, targetIDStr)

	return r
}
