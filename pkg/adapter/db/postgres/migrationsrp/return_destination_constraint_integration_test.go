//go:build integration

package migrationsrp_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
)

func TestReturnDestinationConstraints_PostgreSQLEnforcesGrammar(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t, ctx)
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	merchantID := uuid.New()
	merchantProfileID := seedMerchantProfile(t, ctx, tx, merchantID)
	branchProfileID := seedOnlineBranchProfile(t, ctx, tx, merchantID)

	validName := "damz-web"
	require.NoError(t, returndestination.ValidateDestinationName(validName))

	_, err = tx.Exec(
		ctx,
		`
INSERT INTO bnpl.merchant_return_destinations (
	merchant_id, destination_name, return_url, enabled
) VALUES ($1, $2, $3, true)`,
		merchantID,
		validName,
		"https://merchant.example/return",
	)
	require.NoError(t, err, "valid destination_name must insert")

	_, err = tx.Exec(
		ctx,
		`
INSERT INTO bnpl.checkout_sessions (
	merchant_profile_id,
	merchant_branch_profile_id,
	online_store_id,
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
	offer_preparation_mode
) VALUES (
	$1, $2, $3, $4, $5, '',
	'cs_' || md5(random()::text) || md5(random()::text), 'external_partner',
	$6, $7, '', 1, 'IRR', 'open', now() + interval '1 hour', 1, 'on_start'
)`,
		merchantProfileID,
		branchProfileID,
		uuid.New(),
		"idem-"+uuid.NewString(),
		"order-"+uuid.NewString(),
		validName,
		"https://merchant.example/return",
	)
	require.NoError(t, err, "valid return_destination must insert")

	invalidNames := []string{
		"Foo",
		"foo_bar",
		"foo--bar",
		"-damz",
		"damz-",
	}

	for _, name := range invalidNames {
		t.Run("merchant_return_destinations/"+name, func(t *testing.T) {
			insertErr := execInSavepoint(
				t,
				ctx,
				tx,
				`
INSERT INTO bnpl.merchant_return_destinations (
	merchant_id, destination_name, return_url, enabled
) VALUES ($1, $2, $3, true)`,
				merchantID,
				name,
				"https://merchant.example/return",
			)
			requireCheckViolation(t, insertErr)
		})

		t.Run("checkout_sessions/"+name, func(t *testing.T) {
			insertErr := execInSavepoint(
				t,
				ctx,
				tx,
				`
INSERT INTO bnpl.checkout_sessions (
	merchant_profile_id,
	merchant_branch_profile_id,
	online_store_id,
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
	offer_preparation_mode
) VALUES (
	$1, $2, $3, $4, $5, '',
	'cs_' || md5(random()::text) || md5(random()::text), 'external_partner',
	$6, $7, '', 1, 'IRR', 'open', now() + interval '1 hour', 1, 'on_start'
)`,
				merchantProfileID,
				branchProfileID,
				uuid.New(),
				"idem-"+uuid.NewString(),
				"order-"+uuid.NewString(),
				name,
				"https://merchant.example/return",
			)
			requireCheckViolation(t, insertErr)
		})
	}
}

func TestReturnDestinationConstraints_CoreAndDatabaseAgree(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t, ctx)
	defer pool.Close()

	cases := []struct {
		name  string
		valid bool
	}{
		{name: "default", valid: true},
		{name: "damz-web", valid: true},
		{name: "a", valid: true},
		{name: "a1", valid: true},
		{name: "storefront-1", valid: true},
		{name: "Damz", valid: false},
		{name: "damz_web", valid: false},
		{name: "damz--web", valid: false},
		{name: "-damz", valid: false},
		{name: "damz-", valid: false},
		{name: "foo/bar", valid: false},
		{name: "foo bar", valid: false},
		{name: "سلام", valid: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			coreErr := returndestination.ValidateDestinationName(tc.name)

			var dbMatch bool
			err := pool.QueryRow(ctx, `SELECT $1::text ~ $2::text`, tc.name, returndestination.DestinationNamePattern).Scan(&dbMatch)
			require.NoError(t, err)

			if tc.valid {
				require.NoError(t, coreErr)
				require.True(t, dbMatch)
				return
			}

			require.Error(t, coreErr)
			require.False(t, dbMatch)
		})
	}
}

func openIntegrationPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	dsn := postgresDSNFromEnv()
	if dsn == "" {
		t.Skip("POSTGRES_* env not configured; skipping PostgreSQL constraint integration test")
	}

	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(ctx))

	return pool
}

func postgresDSNFromEnv() string {
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

func seedMerchantProfile(t *testing.T, ctx context.Context, tx pgx.Tx, merchantID uuid.UUID) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := tx.QueryRow(ctx, `
INSERT INTO bnpl.merchant_profiles (
	merchant_id, display_name, status, risk_tier, max_tenure_months, jurisdiction
) VALUES ($1, 'constraint-test', 'active', 'low', 12, 'IR')
RETURNING id`, merchantID).Scan(&id)
	require.NoError(t, err)

	return id
}

func seedOnlineBranchProfile(t *testing.T, ctx context.Context, tx pgx.Tx, merchantID uuid.UUID) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := tx.QueryRow(ctx, `
INSERT INTO bnpl.merchant_branch_profiles (
	merchant_id, branch_id, branch_type, status, jurisdiction
) VALUES ($1, $2, 'online', 'active', 'IR')
RETURNING id`, merchantID, uuid.New()).Scan(&id)
	require.NoError(t, err)

	return id
}

func execInSavepoint(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	sql string,
	args ...any,
) error {
	t.Helper()

	sp := "sp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err := tx.Exec(ctx, "SAVEPOINT "+sp)
	require.NoError(t, err)

	_, err = tx.Exec(ctx, sql, args...)
	if err != nil {
		_, rbErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT "+sp)
		require.NoError(t, rbErr)

		return err
	}

	_, relErr := tx.Exec(ctx, "RELEASE SAVEPOINT "+sp)
	require.NoError(t, relErr)

	return nil
}

func requireCheckViolation(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)

	var pgErr *pgconn.PgError
	require.Truef(
		t,
		errors.As(err, &pgErr),
		"expected pgconn.PgError, got %T: %v",
		err,
		err,
	)
	require.Equal(t, "23514", pgErr.Code, "expected check_violation")
}
