//go:build integration

package decisionrp_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCollectionsDecisionFingerprint_PostgreSQLAcceptsHashedForm(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t, ctx)
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	hasher := sha256fingerprint.New()
	openValue, err := hasher.Hash("collections-default-open:v1:test|" + uuid.NewString())
	require.NoError(t, err)
	require.NoError(t, openValue.Validate())

	recoverValue, err := hasher.Hash("collections-default-recover:v1:test|" + uuid.NewString())
	require.NoError(t, err)
	require.NoError(t, recoverValue.Validate())

	targetID := uuid.New()
	now := time.Now().UTC()

	var openDecisionID uuid.UUID
	err = tx.QueryRow(
		ctx,
		`
INSERT INTO decisions (
	type, target_type, target_id, outcome, reason_codes,
	policy_id, policy_version, jurisdiction, actor_type,
	decided_at, created_at, idempotency_key, idempotency_fingerprint
) VALUES (
	'restriction', 'customer_profile', $1, 'restricted', ARRAY['COLLECTIONS_GRACE_PERIOD_EXCEEDED'],
	'collections_default', 'v1', 'IR', 'system',
	$2, $2, $3, $4
)
RETURNING id`,
		targetID,
		now,
		"collections_default_open:"+uuid.NewString(),
		openValue.String(),
	).Scan(&openDecisionID)
	require.NoError(t, err, "hashed open fingerprint must satisfy decisions CHECK")

	var recoverDecisionID uuid.UUID
	err = tx.QueryRow(
		ctx,
		`
INSERT INTO decisions (
	type, target_type, target_id, outcome, reason_codes,
	policy_id, policy_version, jurisdiction, actor_type,
	prior_decision_id,
	decided_at, created_at, idempotency_key, idempotency_fingerprint
) VALUES (
	'restriction', 'customer_profile', $1, 'unrestricted', ARRAY['COLLECTIONS_DUE_INSTALLMENTS_SETTLED'],
	'collections_default', 'v1', 'IR', 'system',
	$2,
	$3, $3, $4, $5
)
RETURNING id`,
		targetID,
		openDecisionID,
		now,
		"collections_default_recover:"+uuid.NewString(),
		recoverValue.String(),
	).Scan(&recoverDecisionID)
	require.NoError(t, err, "hashed recover fingerprint must satisfy decisions CHECK")

	var prior *uuid.UUID
	err = tx.QueryRow(
		ctx,
		`SELECT prior_decision_id FROM decisions WHERE id = $1`,
		recoverDecisionID,
	).Scan(&prior)
	require.NoError(t, err)
	require.NotNil(t, prior)
	require.Equal(t, openDecisionID, *prior)

	_, err = tx.Exec(
		ctx,
		`
INSERT INTO decisions (
	type, target_type, target_id, outcome, reason_codes,
	policy_id, policy_version, jurisdiction, actor_type,
	decided_at, created_at, idempotency_key, idempotency_fingerprint
) VALUES (
	'restriction', 'customer_profile', $1, 'restricted', ARRAY['COLLECTIONS_GRACE_PERIOD_EXCEEDED'],
	'collections_default', 'v1', 'IR', 'system',
	$2, $2, $3, $4
)`,
		targetID,
		now,
		"collections_default_open_bad:"+uuid.NewString(),
		fmt.Sprintf("collections_default|v1|%s|[]", model.CustomerID(targetID).String()),
	)
	require.Error(t, err, "raw fmt.Sprintf fingerprint must fail decisions CHECK")
}

func openIntegrationPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	dsn := postgresDSNFromEnv()
	if dsn == "" {
		t.Fatal("POSTGRES_* env not configured; skipping PostgreSQL collections fingerprint integration test")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("postgres unavailable: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("postgres unavailable: %v", err)
	}

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
