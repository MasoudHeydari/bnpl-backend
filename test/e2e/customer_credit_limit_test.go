//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

const customerCreditLimitsPath = "/api/v1/customer-credit/initiate"

// Confirmed from pkg/core/policy/initialcreditlimit/v1 and the external credit
// assessment noop provider used in test/e2e: assignment fetches external
// signals then applies the code-owned initial credit-limit policy.
const (
	staticInitialLimitAmountMinor = 90_000_000
	staticInitialLimitCurrency    = "IRR"
	staticInitialLimitValidFor    = 365 * 24 * time.Hour
	staticInitialLimitPolicyID    = "customer_default_initial_limit"
	staticInitialLimitReason      = "INITIAL_CREDIT_LIMIT_POLICY_APPROVED"
)

// -----------------------------------------------------------------------------
// Response DTOs
// -----------------------------------------------------------------------------

type creditLimitDTO struct {
	AmountMinor   uint64     `json:"amount_minor"`
	Currency      string     `json:"currency"`
	Status        string     `json:"status"`
	EffectiveFrom time.Time  `json:"effective_from"`
	ExpiresAt     *time.Time `json:"expires_at"`
}

type creditLimitDecisionDTO struct {
	Outcome     string   `json:"outcome"`
	ReasonCodes []string `json:"reason_codes"`
}

type assignInitialLimitDataDTO struct {
	Result              string                 `json:"result"`
	CreditLimitDecision creditLimitDecisionDTO `json:"credit_limit_decision"`
	CreditLimit         *creditLimitDTO        `json:"credit_limit"`
}

// -----------------------------------------------------------------------------
// Request construction and auth headers
// -----------------------------------------------------------------------------

// assignInitialLimit POSTs with no body. The customer identity is identified by
// x-auth-subject-id in headers (SubjectID == customer_profiles.external_customer_id).
func assignInitialLimit(t *testing.T, headers map[string]string) *http.Response {
	t.Helper()
	return doRequest(
		t,
		http.MethodPost,
		customerCreditLimitsPath,
		headers,
		nil,
	)
}

// -----------------------------------------------------------------------------
// Customer setup
// -----------------------------------------------------------------------------

// activeCustomerProfile is a profile inserted directly into the database, ready
// for initial credit assignment.
type activeCustomerProfile struct {
	ExternalCustomerID model.ExternalCustomerID
	CustomerID         model.CustomerID
}

func createActiveCustomerProfile(t *testing.T) activeCustomerProfile {
	t.Helper()

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := insertCustomerProfile(t, externalCustomerID, model.ActiveCustomerStatus)
	return activeCustomerProfile{
		ExternalCustomerID: externalCustomerID,
		CustomerID:         customerID,
	}
}

// -----------------------------------------------------------------------------
// Direct database access
// -----------------------------------------------------------------------------

type creditAccountRow struct {
	ID                  uuid.UUID
	CustomerID          model.CustomerID
	LastLimitDecisionID model.DecisionID
	AmountMinor         uint64
	ExposureAmountMinor uint64
	Currency            string
	Status              string
	EffectiveFrom       time.Time
	ExpiresAt           *time.Time
	RevokedAt           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type creditTransactionRow struct {
	ID              uuid.UUID
	CreditAccountID uuid.UUID
	Type            string
	AmountMinor     uint64
	Currency        string
	ReasonCode      string
	ReferenceType   string
	ReferenceID     uuid.UUID
}

func queryCreditAccountsByCustomerID(t *testing.T, customerID model.CustomerID) []creditAccountRow {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := testDB.Query(ctx, `
		SELECT id, customer_id, last_limit_decision_id, limit_amount_minor, exposure_amount_minor, currency,
		       status, effective_from, expires_at, revoked_at, created_at, updated_at
		FROM bnpl.credit_accounts
		WHERE customer_id = $1
		ORDER BY created_at ASC, id ASC
	`, customerID)
	require.NoError(t, err)
	defer rows.Close()

	results := make([]creditAccountRow, 0)
	for rows.Next() {
		var idStr, customerIDStr, decisionIDStr string
		var r creditAccountRow

		require.NoError(t, rows.Scan(
			&idStr,
			&customerIDStr,
			&decisionIDStr,
			&r.AmountMinor,
			&r.ExposureAmountMinor,
			&r.Currency,
			&r.Status,
			&r.EffectiveFrom,
			&r.ExpiresAt,
			&r.RevokedAt,
			&r.CreatedAt,
			&r.UpdatedAt,
		))

		r.ID = assertValidUUID(t, idStr)
		r.CustomerID = model.CustomerID(assertValidUUID(t, customerIDStr))
		r.LastLimitDecisionID = model.DecisionID(assertValidUUID(t, decisionIDStr))
		results = append(results, r)
	}
	require.NoError(t, rows.Err())

	return results
}

func queryCreditTransactionsByAccountID(t *testing.T, accountID uuid.UUID) []creditTransactionRow {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := testDB.Query(ctx, `
		SELECT id, credit_account_id, type, amount_minor, currency, reason_code, reference_type, reference_id
		FROM bnpl.credit_transactions
		WHERE credit_account_id = $1
		ORDER BY occurred_at ASC, id ASC
	`, accountID)
	require.NoError(t, err)
	defer rows.Close()

	results := make([]creditTransactionRow, 0)
	for rows.Next() {
		var idStr, accountIDStr, referenceIDStr string
		var r creditTransactionRow
		require.NoError(t, rows.Scan(
			&idStr,
			&accountIDStr,
			&r.Type,
			&r.AmountMinor,
			&r.Currency,
			&r.ReasonCode,
			&r.ReferenceType,
			&referenceIDStr,
		))
		r.ID = assertValidUUID(t, idStr)
		r.CreditAccountID = assertValidUUID(t, accountIDStr)
		r.ReferenceID = assertValidUUID(t, referenceIDStr)
		results = append(results, r)
	}
	require.NoError(t, rows.Err())

	return results
}

func countAllCreditAccounts(t *testing.T) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	require.NoError(t, testDB.QueryRow(ctx, `SELECT count(*) FROM bnpl.credit_accounts`).Scan(&count))

	return count
}

func countActiveCreditAccountsByCustomerID(t *testing.T, customerID model.CustomerID) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	require.NoError(t, testDB.QueryRow(
		ctx,
		`SELECT count(*) FROM bnpl.credit_accounts WHERE customer_id = $1 AND status = 'active'`,
		customerID,
	).Scan(&count))

	return count
}

func countDecisionsByTargetAndType(t *testing.T, targetType string, targetID uuid.UUID, decisionType string) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	require.NoError(t, testDB.QueryRow(
		ctx,
		`SELECT count(*) FROM bnpl.decisions WHERE target_type = $1 AND target_id = $2 AND type = $3`,
		targetType,
		targetID,
		decisionType,
	).Scan(&count))

	return count
}

// assertCreditLimitDenialDecision verifies the complete persisted decision
// contract for a denied credit-limit assignment. Reason codes are compared as
// a set (sorted) since the API contract does not promise ordering.
func assertCreditLimitDenialDecision(t *testing.T, customerID model.CustomerID, wantReasonCodes []string) {
	t.Helper()

	decisions := queryCreditLimitDecisionsByCustomerID(t, customerID)
	require.Len(t, decisions, 1)
	decision := decisions[0]
	assert.Equal(t, "credit_limit", decision.Type)
	assert.Equal(t, "customer_profile", decision.TargetType)
	assert.Equal(t, customerID.UUID(), decision.TargetID)
	assert.Equal(t, "denied", decision.Outcome)

	gotReasonCodes := append([]string(nil), decision.ReasonCodes...)
	sort.Strings(gotReasonCodes)
	wantSorted := append([]string(nil), wantReasonCodes...)
	sort.Strings(wantSorted)
	assert.Equal(t, wantSorted, gotReasonCodes, "exact reason code set must match")

	assert.Equal(t, staticInitialLimitPolicyID, decision.PolicyID)
	assert.Equal(t, "v1", decision.PolicyVersion)
}

func queryCreditLimitDecisionsByCustomerID(t *testing.T, customerID model.CustomerID) []decisionRow {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := testDB.Query(ctx, `
		SELECT id, type, target_type, target_id, outcome, reason_codes, policy_id, policy_version
		FROM bnpl.decisions
		WHERE target_type = 'customer_profile' AND target_id = $1 AND type = 'credit_limit'
		ORDER BY created_at ASC, id ASC
	`, customerID)
	require.NoError(t, err)
	defer rows.Close()

	results := make([]decisionRow, 0)
	for rows.Next() {
		var idStr, targetIDStr string
		var r decisionRow
		require.NoError(t, rows.Scan(
			&idStr,
			&r.Type,
			&r.TargetType,
			&targetIDStr,
			&r.Outcome,
			&r.ReasonCodes,
			&r.PolicyID,
			&r.PolicyVersion,
		))
		r.ID = assertValidUUID(t, idStr)
		r.TargetID = assertValidUUID(t, targetIDStr)
		results = append(results, r)
	}
	require.NoError(t, rows.Err())
	return results
}

// expireCreditAccount forces expires_at on an existing credit account into the past
// and marks it expired so a refresh path is eligible.
func expireCreditAccount(t *testing.T, creditAccountID uuid.UUID, expiresAt time.Time) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := testDB.Exec(ctx, `
		UPDATE bnpl.credit_accounts
		SET expires_at = $2, status = 'expired', updated_at = now()
		WHERE id = $1
	`, creditAccountID, expiresAt)
	require.NoError(t, err)
}

// setCustomerProfileStatus forces a customer profile's lifecycle status.
func setCustomerProfileStatus(t *testing.T, customerID model.CustomerID, status model.CustomerStatus) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := testDB.Exec(ctx, `
		UPDATE bnpl.customer_profiles
		SET status = $2, updated_at = now()
		WHERE id = $1
	`, customerID, string(status))
	require.NoError(t, err)
}

// -----------------------------------------------------------------------------
// 1. Successful initial credit limit assignment
// -----------------------------------------------------------------------------

func TestE2E_CustomerCreditLimit_Create_Succeeds(t *testing.T) {
	resetDatabase(t)

	bp := createActiveCustomerProfile(t)

	response := assignInitialLimit(t, creditCreateAuthHeaders(bp.ExternalCustomerID))
	require.Equal(t, http.StatusCreated, response.StatusCode)

	data, meta := decodeSuccess[assignInitialLimitDataDTO](t, response)

	assert.Equal(t, "CUSTOMER_INITIAL_CREDIT_LIMIT_ASSIGNED", meta.Code)
	assert.Equal(t, http.StatusCreated, meta.StatusCode)
	assert.NotEmpty(t, meta.TraceID)
	assert.NotEmpty(t, meta.RequestID)

	assert.Equal(t, "created", data.Result)

	decision := data.CreditLimitDecision
	assert.Equal(t, "limit_set", decision.Outcome)
	assert.Contains(t, decision.ReasonCodes, staticInitialLimitReason)

	require.NotNil(t, data.CreditLimit)
	assert.Equal(t, uint64(staticInitialLimitAmountMinor), data.CreditLimit.AmountMinor)
	assert.Equal(t, staticInitialLimitCurrency, data.CreditLimit.Currency)
	assert.Equal(t, "active", data.CreditLimit.Status)
	assertNonZeroTime(t, data.CreditLimit.EffectiveFrom)
	require.NotNil(t, data.CreditLimit.ExpiresAt)
	assert.True(t, data.CreditLimit.ExpiresAt.After(data.CreditLimit.EffectiveFrom))

	assertTraceHeadersGenerated(t, response.Header)

	rows := queryCreditAccountsByCustomerID(t, bp.CustomerID)
	require.Len(t, rows, 1, "exactly one credit account row must be persisted")
	row := rows[0]
	accountID := row.ID
	decisionID := row.LastLimitDecisionID
	assert.Equal(t, uint64(staticInitialLimitAmountMinor), row.AmountMinor)
	assert.Equal(t, uint64(0), row.ExposureAmountMinor)
	assert.Equal(t, staticInitialLimitCurrency, row.Currency)
	assert.Equal(t, "active", row.Status)
	require.NotNil(t, row.ExpiresAt)
	assert.True(t, row.ExpiresAt.After(row.EffectiveFrom))
	assert.Nil(t, row.RevokedAt)

	persistedDecision := queryDecisionByID(t, decisionID)
	assert.Equal(t, "credit_limit", persistedDecision.Type)
	assert.Equal(t, "limit_set", persistedDecision.Outcome)
	assert.Equal(t, bp.CustomerID.UUID(), persistedDecision.TargetID)
	assert.Equal(t, staticInitialLimitPolicyID, persistedDecision.PolicyID)
	assert.Contains(t, persistedDecision.ReasonCodes, staticInitialLimitReason)

	profile, found := queryCustomerProfile(t, bp.ExternalCustomerID)
	require.True(t, found)
	assert.Equal(t, "active", profile.Status)

	txs := queryCreditTransactionsByAccountID(t, accountID)
	require.Len(t, txs, 1)
	assert.Equal(t, "limit_set", txs[0].Type)
	assert.Equal(t, uint64(staticInitialLimitAmountMinor), txs[0].AmountMinor)
	assert.Equal(t, "decision", txs[0].ReferenceType)
	assert.Equal(t, decisionID, txs[0].ReferenceID)

	assert.Equal(t, 1, countAllCreditAccounts(t))
	assert.Equal(t, 1, countActiveCreditAccountsByCustomerID(t, bp.CustomerID))
	assert.Equal(t, 1, countDecisionsByTargetAndType(t, "customer_profile", bp.CustomerID.UUID(), "credit_limit"))
}

// -----------------------------------------------------------------------------
// 2. Missing profile is auto-created
// -----------------------------------------------------------------------------

func TestE2E_CustomerCreditLimit_MissingProfile_IsCreatedAndAssigned(t *testing.T) {
	resetDatabase(t)

	subjectID := model.ExternalCustomerID(uuid.New())
	require.Equal(t, 0, countAllCustomerProfiles(t))

	response := assignInitialLimit(t, creditCreateAuthHeaders(subjectID))
	require.Equal(t, http.StatusCreated, response.StatusCode)

	data, _ := decodeSuccess[assignInitialLimitDataDTO](t, response)
	assert.Equal(t, "created", data.Result)
	require.NotNil(t, data.CreditLimit)
	assert.Equal(t, uint64(staticInitialLimitAmountMinor), data.CreditLimit.AmountMinor)
	assert.Equal(t, staticInitialLimitCurrency, data.CreditLimit.Currency)
	assert.Equal(t, "active", data.CreditLimit.Status)
	assert.Equal(t, "limit_set", data.CreditLimitDecision.Outcome)

	profile, found := queryCustomerProfile(t, subjectID)
	require.True(t, found, "missing profile must be created")
	assert.NotEqual(t, uuid.Nil, profile.ID)
	assert.Equal(t, subjectID, profile.ExternalCustomerID)
	assert.Equal(t, "active", profile.Status)
	assert.Equal(t, "IR", profile.Jurisdiction)

	assert.Equal(t, 1, countAllCustomerProfiles(t))
	assert.Equal(t, 1, countActiveCreditAccountsByCustomerID(t, model.CustomerID(profile.ID)))
}

func TestE2E_CustomerCreditLimit_MissingProfile_ConcurrentRequests_CreateOneProfile(t *testing.T) {
	resetDatabase(t)

	subjectID := model.ExternalCustomerID(uuid.New())
	headers := creditCreateAuthHeaders(subjectID)

	const concurrency = 8

	var wg sync.WaitGroup
	statusCodes := make([]int, concurrency)
	responseBodies := make([][]byte, concurrency)
	requestErrs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			request, reqErr := http.NewRequest(http.MethodPost, e2eBaseURL+customerCreditLimitsPath, nil)
			if reqErr != nil {
				requestErrs[idx] = reqErr
				return
			}

			for name, value := range headers {
				request.Header.Set(name, value)
			}

			response, doErr := e2eClient.Do(request)
			if doErr != nil {
				requestErrs[idx] = doErr
				return
			}

			raw, readErr := readAndClose(response)
			if readErr != nil {
				requestErrs[idx] = readErr
				return
			}

			statusCodes[idx] = response.StatusCode
			responseBodies[idx] = raw
		}(i)
	}
	wg.Wait()

	for i, err := range requestErrs {
		require.NoErrorf(t, err, "concurrent request %d failed", i)
	}

	var createdCount, alreadyAssignedCount int
	for i, code := range statusCodes {
		var envelope successEnvelope[assignInitialLimitDataDTO]
		require.NoErrorf(
			t,
			json.Unmarshal(responseBodies[i], &envelope),
			"decode response %d: %s",
			i,
			responseBodies[i],
		)
		require.NotNilf(t, envelope.Data.CreditLimit, "response %d must carry a credit limit", i)

		switch {
		case code == http.StatusCreated && envelope.Data.Result == "created":
			createdCount++

		case code == http.StatusOK && envelope.Data.Result == "already_assigned":
			alreadyAssignedCount++

		default:
			t.Fatalf(
				"response %d: status %d must pair with result \"created\" (201) or \"already_assigned\" (200), got result %q (body: %s)",
				i,
				code,
				envelope.Data.Result,
				responseBodies[i],
			)
		}
	}

	assert.Equal(t, 1, createdCount)
	assert.Equal(t, concurrency-1, alreadyAssignedCount)
	assert.Equal(t, 1, countAllCustomerProfiles(t), "exactly one customer profile must be created")
	assert.Equal(t, 1, countAllCreditAccounts(t))

	profile, found := queryCustomerProfile(t, subjectID)
	require.True(t, found)
	assert.NotEqual(t, uuid.Nil, profile.ID)
	assert.Equal(t, subjectID, profile.ExternalCustomerID)
	assert.Equal(t, "active", profile.Status)
	assert.Equal(t, "IR", profile.Jurisdiction)
	assert.Equal(t, 1, countActiveCreditAccountsByCustomerID(t, model.CustomerID(profile.ID)))
}

func TestE2E_CustomerCreditLimit_MissingProfile_IdempotentRetry(t *testing.T) {
	resetDatabase(t)

	subjectID := model.ExternalCustomerID(uuid.New())
	headers := creditCreateAuthHeaders(subjectID)

	first := assignInitialLimit(t, headers)
	require.Equal(t, http.StatusCreated, first.StatusCode)
	firstData, _ := decodeSuccess[assignInitialLimitDataDTO](t, first)
	assert.Equal(t, "created", firstData.Result)

	second := assignInitialLimit(t, headers)
	require.Equal(t, http.StatusOK, second.StatusCode)
	secondData, _ := decodeSuccess[assignInitialLimitDataDTO](t, second)
	assert.Equal(t, "already_assigned", secondData.Result)

	profile, found := queryCustomerProfile(t, subjectID)
	require.True(t, found)

	assert.Equal(t, 1, countAllCustomerProfiles(t))
	assert.Equal(t, 1, countAllCreditAccounts(t))
	assert.Equal(t, 1, countActiveCreditAccountsByCustomerID(t, model.CustomerID(profile.ID)))
}

// -----------------------------------------------------------------------------
// 3. Customer profile status restrictions
// -----------------------------------------------------------------------------

func TestE2E_CustomerCreditLimit_NonActiveProfile_IsDenied(t *testing.T) {
	tests := []struct {
		name   string
		status model.CustomerStatus
	}{
		{"deactivated", model.DeactivatedCustomerStatus},
		{"terminated", model.TerminatedCustomerStatus},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDatabase(t)

			bp := createActiveCustomerProfile(t)
			setCustomerProfileStatus(t, bp.CustomerID, tt.status)

			response := assignInitialLimit(t, creditCreateAuthHeaders(bp.ExternalCustomerID))
			require.Equal(t, http.StatusOK, response.StatusCode)

			data, _ := decodeSuccess[assignInitialLimitDataDTO](t, response)
			assert.Equal(t, "denied", data.Result)
			assert.Nil(t, data.CreditLimit)
			assert.Equal(t, "denied", data.CreditLimitDecision.Outcome)
			assert.Contains(t, data.CreditLimitDecision.ReasonCodes, "CUSTOMER_PROFILE_NOT_ACTIVE")

			assertCreditLimitDenialDecision(t, bp.CustomerID, []string{"CUSTOMER_PROFILE_NOT_ACTIVE"})

			assert.Equal(t, 0, countAllCreditAccounts(t))
			assert.Equal(t, 1, countDecisionsByTargetAndType(t, "customer_profile", bp.CustomerID.UUID(), "credit_limit"))

			profile, found := queryCustomerProfile(t, bp.ExternalCustomerID)
			require.True(t, found)
			assert.Equal(t, string(tt.status), profile.Status, "the profile status must remain unchanged")
		})
	}
}

// -----------------------------------------------------------------------------
// 4. Sequential repeated request
// -----------------------------------------------------------------------------

func TestE2E_CustomerCreditLimit_RepeatedRequest_IsIdempotent(t *testing.T) {
	resetDatabase(t)

	bp := createActiveCustomerProfile(t)

	first := assignInitialLimit(t, creditCreateAuthHeaders(bp.ExternalCustomerID))
	require.Equal(t, http.StatusCreated, first.StatusCode)
	firstData, _ := decodeSuccess[assignInitialLimitDataDTO](t, first)
	require.Equal(t, "created", firstData.Result)
	require.NotNil(t, firstData.CreditLimit)

	second := assignInitialLimit(t, creditCreateAuthHeaders(bp.ExternalCustomerID))
	require.Equal(t, http.StatusOK, second.StatusCode, "a repeated assignment while the limit is still active must be idempotent")
	secondData, secondMeta := decodeSuccess[assignInitialLimitDataDTO](t, second)
	assert.Equal(t, "already_assigned", secondData.Result)
	assert.Equal(t, "CUSTOMER_INITIAL_CREDIT_LIMIT_ASSIGNED", secondMeta.Code)

	require.NotNil(t, secondData.CreditLimit)
	assert.Equal(t, firstData.CreditLimit.AmountMinor, secondData.CreditLimit.AmountMinor)
	assert.Equal(t, firstData.CreditLimit.Currency, secondData.CreditLimit.Currency)
	assert.Equal(t, firstData.CreditLimit.Status, secondData.CreditLimit.Status)
	assert.True(t, firstData.CreditLimit.EffectiveFrom.Equal(secondData.CreditLimit.EffectiveFrom))

	assert.Equal(t, 1, countAllCreditAccounts(t), "exactly one credit account must exist")
	assert.Equal(t, 1, countActiveCreditAccountsByCustomerID(t, bp.CustomerID), "exactly one active credit account must exist")
	assert.Equal(t, 1, countDecisionsByTargetAndType(t, "customer_profile", bp.CustomerID.UUID(), "credit_limit"), "no duplicate credit limit decision may be created")

	rows := queryCreditAccountsByCustomerID(t, bp.CustomerID)
	require.Len(t, rows, 1)
	assert.Equal(t, "active", rows[0].Status)
	require.Len(t, queryCreditTransactionsByAccountID(t, rows[0].ID), 1)
}

// -----------------------------------------------------------------------------
// 5. Refresh after expiration
// -----------------------------------------------------------------------------

func TestE2E_CustomerCreditLimit_ExpiredLimit_IsRefreshed(t *testing.T) {
	resetDatabase(t)

	bp := createActiveCustomerProfile(t)

	first := assignInitialLimit(t, creditCreateAuthHeaders(bp.ExternalCustomerID))
	require.Equal(t, http.StatusCreated, first.StatusCode)
	firstData, _ := decodeSuccess[assignInitialLimitDataDTO](t, first)
	require.NotNil(t, firstData.CreditLimit)

	firstRows := queryCreditAccountsByCustomerID(t, bp.CustomerID)
	require.Len(t, firstRows, 1)
	accountID := firstRows[0].ID
	oldDecisionID := firstRows[0].LastLimitDecisionID

	expireCreditAccount(t, accountID, time.Now().Add(-1*time.Hour))

	second := assignInitialLimit(t, creditCreateAuthHeaders(bp.ExternalCustomerID))
	require.Equal(t, http.StatusCreated, second.StatusCode, "a fresh assignment must be created once the active limit has expired")
	secondData, _ := decodeSuccess[assignInitialLimitDataDTO](t, second)
	assert.Equal(t, "created", secondData.Result)
	require.NotNil(t, secondData.CreditLimit)

	assert.Equal(t, uint64(staticInitialLimitAmountMinor), secondData.CreditLimit.AmountMinor)
	assert.Equal(t, staticInitialLimitCurrency, secondData.CreditLimit.Currency)
	assert.Equal(t, "active", secondData.CreditLimit.Status)
	assert.Contains(t, secondData.CreditLimitDecision.ReasonCodes, staticInitialLimitReason)

	rows := queryCreditAccountsByCustomerID(t, bp.CustomerID)
	require.Len(t, rows, 1, "refresh must update the single CreditAccount row, not create a second account")
	row := rows[0]
	assert.Equal(t, accountID, row.ID)
	assert.Equal(t, "active", row.Status)
	assert.Equal(t, uint64(0), row.ExposureAmountMinor, "limit refresh must not mutate exposure")
	assert.NotEqual(t, oldDecisionID, row.LastLimitDecisionID, "the refresh must be authorized by its own governed decision")
	assertNonZeroTime(t, row.EffectiveFrom)
	require.NotNil(t, row.ExpiresAt)
	assert.WithinDuration(
		t,
		row.EffectiveFrom.Add(staticInitialLimitValidFor),
		*row.ExpiresAt,
		time.Minute,
		"the refreshed limit's validity window must match the static policy",
	)

	txs := queryCreditTransactionsByAccountID(t, accountID)
	require.Len(t, txs, 2, "initial assignment and refresh must each append a CreditTransaction")
	assert.Equal(t, "limit_set", txs[0].Type)
	assert.Equal(t, oldDecisionID, txs[0].ReferenceID)
	assert.Equal(t, "limit_set", txs[1].Type)
	assert.Equal(t, row.LastLimitDecisionID, txs[1].ReferenceID)

	assert.Equal(t, 1, countActiveCreditAccountsByCustomerID(t, bp.CustomerID), "exactly one current account may exist after a refresh")
	assert.Equal(t, 2, countDecisionsByTargetAndType(t, "customer_profile", bp.CustomerID.UUID(), "credit_limit"), "exactly two decisions: the original assignment and the refresh")
}

// -----------------------------------------------------------------------------
// 6. Authorization: permission bnpl:credit-limits:create:self required
// -----------------------------------------------------------------------------

func TestE2E_CustomerCreditLimit_RequiresCreditCreatePermission(t *testing.T) {
	t.Run("wrong permission is forbidden", func(t *testing.T) {
		resetDatabase(t)

		bp := createActiveCustomerProfile(t)
		headers := authHeaders(model.SubjectID(bp.ExternalCustomerID.UUID()), permission.DecisionsCreateSelf.Code)
		response := assignInitialLimit(t, headers)
		require.Equal(t, http.StatusForbidden, response.StatusCode)

		errResp := decodeError(t, response)
		assert.Equal(t, "FORBIDDEN", errResp.Code)

		assert.Equal(t, 0, countAllCreditAccounts(t))
		assert.Equal(t, 0, countDecisionsByTargetAndType(t, "customer_profile", bp.CustomerID.UUID(), "credit_limit"))

		profile, found := queryCustomerProfile(t, bp.ExternalCustomerID)
		require.True(t, found)
		assert.Equal(t, "active", profile.Status)
	})

	t.Run("payments:start:self permission is forbidden", func(t *testing.T) {
		resetDatabase(t)

		bp := createActiveCustomerProfile(t)
		headers := authHeaders(model.SubjectID(bp.ExternalCustomerID.UUID()), permission.PaymentsStartSelf.Code)
		response := assignInitialLimit(t, headers)
		require.Equal(t, http.StatusForbidden, response.StatusCode)

		assert.Equal(t, 0, countAllCreditAccounts(t))
	})

	t.Run("credit-limits:create:self is allowed", func(t *testing.T) {
		resetDatabase(t)

		bp := createActiveCustomerProfile(t)
		response := assignInitialLimit(t, creditCreateAuthHeaders(bp.ExternalCustomerID))
		require.Equal(t, http.StatusCreated, response.StatusCode)

		data, _ := decodeSuccess[assignInitialLimitDataDTO](t, response)
		assert.Equal(t, "created", data.Result)
		require.NotNil(t, data.CreditLimit)
		assert.Equal(t, uint64(staticInitialLimitAmountMinor), data.CreditLimit.AmountMinor)

		assert.Equal(t, 1, countActiveCreditAccountsByCustomerID(t, bp.CustomerID))
		rows := queryCreditAccountsByCustomerID(t, bp.CustomerID)
		require.Len(t, rows, 1)
	})

	t.Run("credit permission among others is allowed", func(t *testing.T) {
		resetDatabase(t)

		bp := createActiveCustomerProfile(t)
		headers := authHeaders(
			model.SubjectID(bp.ExternalCustomerID.UUID()),
			permission.DecisionsCreateSelf.Code,
			permission.CreditLimitsCreateSelf.Code,
		)
		response := assignInitialLimit(t, headers)
		require.Equal(t, http.StatusCreated, response.StatusCode)

		data, _ := decodeSuccess[assignInitialLimitDataDTO](t, response)
		assert.Equal(t, "created", data.Result)
	})
}

// -----------------------------------------------------------------------------
// 7. Missing and invalid authentication headers
// -----------------------------------------------------------------------------

func TestE2E_CustomerCreditLimit_MissingOrInvalidHeaders(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(headers map[string]string) map[string]string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing subject id",
			mutate:     func(h map[string]string) map[string]string { return withoutHeader(h, header.AuthSubjectID) },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "malformed subject id",
			mutate:     func(h map[string]string) map[string]string { return withHeader(h, header.AuthSubjectID, "not-a-uuid") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "blank subject id",
			mutate:     func(h map[string]string) map[string]string { return withHeader(h, header.AuthSubjectID, "   ") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "missing tenant id",
			mutate:     func(h map[string]string) map[string]string { return withoutHeader(h, header.AuthTenantID) },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "malformed tenant id",
			mutate:     func(h map[string]string) map[string]string { return withHeader(h, header.AuthTenantID, "not-a-uuid") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "missing context id",
			mutate:     func(h map[string]string) map[string]string { return withoutHeader(h, header.AuthContextID) },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "malformed context id",
			mutate:     func(h map[string]string) map[string]string { return withHeader(h, header.AuthContextID, "not-a-uuid") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "missing permissions",
			mutate:     func(h map[string]string) map[string]string { return withoutHeader(h, header.AuthPermissions) },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "blank permissions",
			mutate:     func(h map[string]string) map[string]string { return withHeader(h, header.AuthPermissions, "   ") },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name: "authenticated but wrong permission",
			mutate: func(h map[string]string) map[string]string {
				return withHeader(h, header.AuthPermissions, permission.DecisionsCreateSelf.Code)
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "FORBIDDEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDatabase(t)

			bp := createActiveCustomerProfile(t)
			headers := tt.mutate(creditCreateAuthHeaders(bp.ExternalCustomerID))

			response := assignInitialLimit(t, headers)
			require.Equal(t, tt.wantStatus, response.StatusCode)

			errResp := decodeError(t, response)
			assert.Equal(t, tt.wantCode, errResp.Code)

			assert.Equal(t, 0, countAllCreditAccounts(t))
		})
	}
}

// -----------------------------------------------------------------------------
// 8. Request body is ignored (no bind)
// -----------------------------------------------------------------------------

func TestE2E_CustomerCreditLimit_RequestBody_IsIgnored(t *testing.T) {
	tests := []struct {
		name    string
		rawBody []byte
		payload map[string]any
	}{
		{
			name:    "nil body (no content)",
			rawBody: nil,
		},
		{
			name:    "empty body",
			rawBody: []byte{},
		},
		{
			name:    "malformed json is ignored",
			rawBody: []byte(`{"customer_id":`),
		},
		{
			name:    "legacy customer_id body is ignored",
			payload: map[string]any{"customer_id": uuid.NewString()},
		},
		{
			name:    "unrelated json object is ignored",
			payload: map[string]any{"foo": "bar", "amount": 123},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDatabase(t)

			bp := createActiveCustomerProfile(t)
			headers := creditCreateAuthHeaders(bp.ExternalCustomerID)

			var response *http.Response
			switch {
			case tt.payload != nil:
				response = postJSON(t, customerCreditLimitsPath, headers, tt.payload)

			default:
				response = doRequest(
					t,
					http.MethodPost,
					customerCreditLimitsPath,
					headers,
					tt.rawBody,
				)
			}
			require.Equal(t, http.StatusCreated, response.StatusCode)

			data, _ := decodeSuccess[assignInitialLimitDataDTO](t, response)
			assert.Equal(t, "created", data.Result)
			require.NotNil(t, data.CreditLimit)
			assert.Equal(t, uint64(staticInitialLimitAmountMinor), data.CreditLimit.AmountMinor)
			assert.Equal(t, 1, countActiveCreditAccountsByCustomerID(t, bp.CustomerID))
		})
	}
}

// -----------------------------------------------------------------------------
// 10. Trace propagation
// -----------------------------------------------------------------------------

func TestE2E_CustomerCreditLimit_TraceHeaders_GeneratedWhenAbsent(t *testing.T) {
	resetDatabase(t)

	bp := createActiveCustomerProfile(t)
	response := assignInitialLimit(t, creditCreateAuthHeaders(bp.ExternalCustomerID))
	require.Equal(t, http.StatusCreated, response.StatusCode)
	drainAndClose(response)

	assertTraceHeadersGenerated(t, response.Header)
}

func TestE2E_CustomerCreditLimit_TraceHeaders_PreservedWhenSupplied(t *testing.T) {
	resetDatabase(t)

	bp := createActiveCustomerProfile(t)
	headers := withHeader(
		withHeader(
			withHeader(creditCreateAuthHeaders(bp.ExternalCustomerID), header.TraceID, "client-trace-id"),
			header.RequestID, "client-request-id",
		),
		header.CorrelationID, "client-correlation-id",
	)

	response := assignInitialLimit(t, headers)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	defer drainAndClose(response)

	assert.Equal(t, "client-trace-id", response.Header.Get(header.TraceID))
	assert.Equal(t, "client-request-id", response.Header.Get(header.RequestID))
	assert.Equal(t, "client-correlation-id", response.Header.Get(header.CorrelationID))
}

// -----------------------------------------------------------------------------
// 11. Concurrency: identical concurrent requests for the same customer
// -----------------------------------------------------------------------------

func TestE2E_CustomerCreditLimit_ConcurrentIdenticalRequests(t *testing.T) {
	resetDatabase(t)

	bp := createActiveCustomerProfile(t)
	headers := creditCreateAuthHeaders(bp.ExternalCustomerID)

	const concurrency = 8

	var wg sync.WaitGroup
	statusCodes := make([]int, concurrency)
	responseBodies := make([][]byte, concurrency)
	requestErrs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			request, reqErr := http.NewRequest(http.MethodPost, e2eBaseURL+customerCreditLimitsPath, nil)
			if reqErr != nil {
				requestErrs[idx] = reqErr
				return
			}

			for name, value := range headers {
				request.Header.Set(name, value)
			}

			response, doErr := e2eClient.Do(request)
			if doErr != nil {
				requestErrs[idx] = doErr
				return
			}

			raw, readErr := readAndClose(response)
			if readErr != nil {
				requestErrs[idx] = readErr
				return
			}

			statusCodes[idx] = response.StatusCode
			responseBodies[idx] = raw
		}(i)
	}
	wg.Wait()

	for i, err := range requestErrs {
		require.NoErrorf(t, err, "concurrent request %d failed", i)
	}

	var createdCount, alreadyAssignedCount int
	amounts := make(map[uint64]struct{})

	for i, code := range statusCodes {
		var envelope successEnvelope[assignInitialLimitDataDTO]
		require.NoErrorf(
			t,
			json.Unmarshal(responseBodies[i], &envelope),
			"decode response %d: %s",
			i,
			responseBodies[i],
		)
		require.NotNilf(t, envelope.Data.CreditLimit, "response %d must carry a credit limit", i)

		switch {
		case code == http.StatusCreated && envelope.Data.Result == "created":
			createdCount++

		case code == http.StatusOK && envelope.Data.Result == "already_assigned":
			alreadyAssignedCount++

		default:
			t.Fatalf(
				"response %d: status %d must pair with result \"created\" (201) or \"already_assigned\" (200), got result %q (body: %s)",
				i,
				code,
				envelope.Data.Result,
				responseBodies[i],
			)
		}

		amounts[envelope.Data.CreditLimit.AmountMinor] = struct{}{}
		assert.Equal(t, "active", envelope.Data.CreditLimit.Status)
		assert.Equal(t, "limit_set", envelope.Data.CreditLimitDecision.Outcome)
	}

	assert.Equal(t, 1, createdCount, "exactly one concurrent request must return 201 Created with result=created")
	assert.Equal(t, concurrency-1, alreadyAssignedCount, "every other concurrent request must return 200 OK with result=already_assigned, never a conflict or failure")
	assert.Len(t, amounts, 1, "all concurrent responses must report the same limit amount")

	assert.Equal(t, 1, countAllCreditAccounts(t), "exactly one credit-account row must be persisted")
	assert.Equal(t, 1, countActiveCreditAccountsByCustomerID(t, bp.CustomerID))
	assert.Equal(t, 1, countDecisionsByTargetAndType(t, "customer_profile", bp.CustomerID.UUID(), "credit_limit"), "exactly one related credit-limit decision must be persisted")

	rows := queryCreditAccountsByCustomerID(t, bp.CustomerID)
	require.Len(t, rows, 1)
	require.Len(t, queryCreditTransactionsByAccountID(t, rows[0].ID), 1)
}

// -----------------------------------------------------------------------------
// 12. Concurrency: identical concurrent refresh requests after expiry
// -----------------------------------------------------------------------------

func TestE2E_CustomerCreditLimit_ConcurrentRefreshRequests(t *testing.T) {
	resetDatabase(t)

	bp := createActiveCustomerProfile(t)

	first := assignInitialLimit(t, creditCreateAuthHeaders(bp.ExternalCustomerID))
	require.Equal(t, http.StatusCreated, first.StatusCode)
	firstData, _ := decodeSuccess[assignInitialLimitDataDTO](t, first)
	require.NotNil(t, firstData.CreditLimit)

	firstRows := queryCreditAccountsByCustomerID(t, bp.CustomerID)
	require.Len(t, firstRows, 1)
	accountID := firstRows[0].ID
	originalDecisionID := firstRows[0].LastLimitDecisionID

	expireCreditAccount(t, accountID, time.Now().Add(-1*time.Hour))

	headers := creditCreateAuthHeaders(bp.ExternalCustomerID)

	const concurrency = 8

	var wg sync.WaitGroup
	statusCodes := make([]int, concurrency)
	responseBodies := make([][]byte, concurrency)
	requestErrs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			request, reqErr := http.NewRequest(http.MethodPost, e2eBaseURL+customerCreditLimitsPath, bytes.NewReader(nil))
			if reqErr != nil {
				requestErrs[idx] = reqErr
				return
			}

			for name, value := range headers {
				request.Header.Set(name, value)
			}

			response, doErr := e2eClient.Do(request)
			if doErr != nil {
				requestErrs[idx] = doErr
				return
			}

			raw, readErr := readAndClose(response)
			if readErr != nil {
				requestErrs[idx] = readErr
				return
			}

			statusCodes[idx] = response.StatusCode
			responseBodies[idx] = raw
		}(i)
	}
	wg.Wait()

	for i, err := range requestErrs {
		require.NoErrorf(t, err, "concurrent refresh request %d failed", i)
	}

	var createdCount, alreadyAssignedCount int

	for i, code := range statusCodes {
		var envelope successEnvelope[assignInitialLimitDataDTO]
		require.NoErrorf(
			t,
			json.Unmarshal(responseBodies[i], &envelope),
			"decode response %d: %s",
			i,
			responseBodies[i],
		)
		require.NotNilf(t, envelope.Data.CreditLimit, "response %d must carry a credit limit", i)

		switch {
		case code == http.StatusCreated && envelope.Data.Result == "created":
			createdCount++

		case code == http.StatusOK && envelope.Data.Result == "already_assigned":
			alreadyAssignedCount++

		default:
			t.Fatalf(
				"response %d: status %d must pair with result \"created\" (201) or \"already_assigned\" (200), got result %q (body: %s)",
				i,
				code,
				envelope.Data.Result,
				responseBodies[i],
			)
		}

		assert.Equal(t, "active", envelope.Data.CreditLimit.Status)
		assert.Equal(t, "limit_set", envelope.Data.CreditLimitDecision.Outcome)
	}

	assert.Equal(t, 1, createdCount, "exactly one concurrent refresh request must return 201 Created with result=created")
	assert.Equal(t, concurrency-1, alreadyAssignedCount, "every other concurrent refresh request must return 200 OK with result=already_assigned")

	rows := queryCreditAccountsByCustomerID(t, bp.CustomerID)
	require.Len(t, rows, 1, "refresh must keep a single CreditAccount row")
	row := rows[0]
	assert.Equal(t, accountID, row.ID)
	assert.Equal(t, "active", row.Status)
	assert.NotEqual(t, originalDecisionID, row.LastLimitDecisionID, "the refresh must be authorized by its own decision, distinct from the original assignment")

	txs := queryCreditTransactionsByAccountID(t, accountID)
	require.Len(t, txs, 2)

	assert.Equal(t, 1, countActiveCreditAccountsByCustomerID(t, bp.CustomerID))
	assert.Equal(t, 2, countDecisionsByTargetAndType(t, "customer_profile", bp.CustomerID.UUID(), "credit_limit"),
		"exactly two decisions: the original assignment and the refresh")

	refreshDecision := queryDecisionByID(t, row.LastLimitDecisionID)
	assert.Equal(t, "credit_limit", refreshDecision.Type)
	assert.Equal(t, "customer_profile", refreshDecision.TargetType)
	assert.Equal(t, bp.CustomerID.UUID(), refreshDecision.TargetID)
	assert.Equal(t, "limit_set", refreshDecision.Outcome, "an approved credit-limit assignment is persisted with the limit_set outcome")
	assert.Equal(t, staticInitialLimitPolicyID, refreshDecision.PolicyID)
	assert.Equal(t, "v1", refreshDecision.PolicyVersion)
	assert.Contains(t, refreshDecision.ReasonCodes, staticInitialLimitReason)
}
