//go:build integration

package checkoutsessionrp_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/checkoutsessionrp"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestReplaceCreditKindAndInvalidateEvaluation_FirstThenReplace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openCheckoutSessionIntegrationPool(t, ctx)
	defer pool.Close()

	pgxTx, err := pool.Begin(ctx)
	require.NoError(t, err)

	defer func() { _ = pgxTx.Rollback(ctx) }()

	tx := &postgres.Tx{Tx: pgxTx}
	sessionID := seedOpenCheckoutWithoutCreditKind(t, ctx, pgxTx)
	now := time.Now().UTC().Truncate(time.Millisecond)

	first, err := checkoutsessionrp.ReplaceCreditKindAndInvalidateEvaluation(
		ctx,
		tx,
		sessionID,
		model.PayInInstallmentsCreditKind,
		1,
		nil,
		now,
	)
	require.NoError(t, err)
	require.Equal(t, model.PayInInstallmentsCreditKind, first.CreditKind)
	require.Equal(t, 1, first.EvaluationGeneration)
	require.Nil(t, first.DecisionID)

	replaced, err := checkoutsessionrp.ReplaceCreditKindAndInvalidateEvaluation(
		ctx,
		tx,
		sessionID,
		model.PayNextMonthCreditKind,
		1,
		nil,
		now.Add(time.Second),
	)
	require.NoError(t, err)
	require.Equal(t, model.PayNextMonthCreditKind, replaced.CreditKind)
	require.Equal(t, 2, replaced.EvaluationGeneration)
	require.Nil(t, replaced.DecisionID)

	_, err = checkoutsessionrp.ReplaceCreditKindAndInvalidateEvaluation(
		ctx,
		tx,
		sessionID,
		model.PayInInstallmentsCreditKind,
		1, // stale generation
		nil,
		now.Add(2*time.Second),
	)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}

func openCheckoutSessionIntegrationPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	dsn := checkoutSessionPostgresDSNFromEnv()
	if dsn == "" {
		t.Skip("POSTGRES_* env not configured; skipping checkout credit-kind integration test")
	}

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))

	return pool
}

func checkoutSessionPostgresDSNFromEnv() string {
	user := strings.TrimSpace(os.Getenv("POSTGRES_USER"))
	password := os.Getenv("POSTGRES_PASSWORD")
	host := strings.TrimSpace(os.Getenv("POSTGRES_HOST"))
	port := strings.TrimSpace(os.Getenv("POSTGRES_PORT"))
	db := strings.TrimSpace(os.Getenv("POSTGRES_DB"))
	sslMode := strings.TrimSpace(os.Getenv("POSTGRES_SSL_MODE"))
	if sslMode == "" {
		sslMode = "disable"
	}

	if user == "" || host == "" || port == "" || db == "" {
		return ""
	}

	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		user,
		password,
		host,
		port,
		db,
		sslMode,
	)
}

func seedOpenCheckoutWithoutCreditKind(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
) model.CheckoutSessionID {
	t.Helper()

	merchantID := uuid.New()
	now := time.Now().UTC()

	var merchantProfileID uuid.UUID
	err := tx.QueryRow(ctx, `
INSERT INTO bnpl.merchant_profiles (
	external_merchant_id, display_name, status, risk_tier, max_tenure_months, jurisdiction
) VALUES ($1, 'credit-kind-itest', 'active', 'low', 12, 'IR')
RETURNING id`, merchantID).Scan(&merchantProfileID)
	require.NoError(t, err)

	var branchProfileID uuid.UUID
	err = tx.QueryRow(ctx, `
INSERT INTO bnpl.merchant_branch_profiles (
	external_merchant_id, external_branch_id, branch_type, status, jurisdiction
) VALUES ($1, $2, 'online', 'active', 'IR')
RETURNING id`, merchantID, uuid.New()).Scan(&branchProfileID)
	require.NoError(t, err)

	var sessionID model.CheckoutSessionID
	err = tx.QueryRow(ctx, `
INSERT INTO bnpl.checkout_sessions (
	merchant_profile_id,
	merchant_branch_profile_id,
	external_online_store_id,
	idempotency_key,
	merchant_order_id,
	customer_reference,
	public_token,
	origin,
	return_destination,
	return_url,
	merchant_state,
	requested_amount_minor,
	currency,
	status,
	expires_at,
	evaluation_generation,
	offer_preparation_mode,
	created_at,
	updated_at
) VALUES (
	$1, $2, $3, $4, $5, '',
	'cs_' || replace($6::text, '-', '') || replace($7::text, '-', ''),
	'external_partner',
	'default', 'https://merchant.example/return', '',
	15000000, 'IRR', 'open', $8, 1, 'on_start', $9, $9
)
RETURNING id`,
		merchantProfileID,
		branchProfileID,
		uuid.New(),
		"idem-"+uuid.NewString(),
		"order-"+uuid.NewString(),
		uuid.New(),
		uuid.New(),
		now.Add(time.Hour),
		now,
	).Scan(&sessionID)
	require.NoError(t, err)

	return sessionID
}
