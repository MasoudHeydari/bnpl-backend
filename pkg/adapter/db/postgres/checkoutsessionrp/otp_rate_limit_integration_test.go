//go:build integration

package checkoutsessionrp_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/checkoutsessionrp"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type concurrentConsumeResult struct {
	err error
}

func TestConsumeOTPRateLimit_ConcurrentFirstBucket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t, ctx)
	defer pool.Close()

	scopeKey := "itest-phone-" + uuid.NewString()
	now := time.Now().UTC().Truncate(time.Millisecond)
	window := 10 * time.Minute

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `
DELETE FROM bnpl.checkout_otp_rate_limits
WHERE scope = $1 AND scope_key = $2`,
			model.CheckoutOTPRateLimitScopePhone, scopeKey,
		)
	})

	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan concurrentConsumeResult, 2)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			result := runConcurrentConsume(
				ctx,
				pool,
				ready,
				start,
				model.CheckoutOTPRateLimitScopePhone,
				scopeKey,
				window,
				2,
				now,
			)
			results <- result
		}()
	}

	for i := 0; i < 2; i++ {
		<-ready
	}

	close(start)
	wg.Wait()
	close(results)

	for result := range results {
		require.NoError(t, result.err)
	}

	var (
		rowCount  int
		sendCount int
	)
	err := pool.QueryRow(ctx, `
SELECT COUNT(*), COALESCE(MAX(send_count), 0)
FROM bnpl.checkout_otp_rate_limits
WHERE scope = $1 AND scope_key = $2`,
		model.CheckoutOTPRateLimitScopePhone, scopeKey,
	).Scan(&rowCount, &sendCount)
	require.NoError(t, err)
	require.Equal(t, 1, rowCount)
	require.Equal(t, 2, sendCount)
}

func TestConsumeOTPRateLimit_ConcurrentMaxOne(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t, ctx)
	defer pool.Close()

	scopeKey := "itest-merchant-" + uuid.NewString()
	now := time.Now().UTC().Truncate(time.Millisecond)
	window := 10 * time.Minute

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `
DELETE FROM bnpl.checkout_otp_rate_limits
WHERE scope = $1 AND scope_key = $2`,
			model.CheckoutOTPRateLimitScopeMerchant, scopeKey,
		)
	})

	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan concurrentConsumeResult, 2)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			result := runConcurrentConsume(
				ctx,
				pool,
				ready,
				start,
				model.CheckoutOTPRateLimitScopeMerchant,
				scopeKey,
				window,
				1,
				now,
			)
			results <- result
		}()
	}

	for i := 0; i < 2; i++ {
		<-ready
	}

	close(start)
	wg.Wait()
	close(results)

	var success, limited int
	for result := range results {
		switch {
		case result.err == nil:
			success++

		case cerr.IsCode(result.err, cerr.CodeTooManyRequests):
			limited++

		default:
			require.NoError(t, result.err)
		}
	}

	require.Equal(t, 1, success)
	require.Equal(t, 1, limited)

	var (
		rowCount  int
		sendCount int
	)
	err := pool.QueryRow(ctx, `
SELECT COUNT(*), COALESCE(MAX(send_count), 0)
FROM bnpl.checkout_otp_rate_limits
WHERE scope = $1 AND scope_key = $2`,
		model.CheckoutOTPRateLimitScopeMerchant, scopeKey,
	).Scan(&rowCount, &sendCount)
	require.NoError(t, err)
	require.Equal(t, 1, rowCount)
	require.Equal(t, 1, sendCount)
}

func TestConsumeOTPRateLimit_MerchantRejectRollsBackPhone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := openIntegrationPool(t, ctx)
	defer pool.Close()

	phoneKey := "itest-phone-rb-" + uuid.NewString()
	merchantKey := "itest-merchant-rb-" + uuid.NewString()
	now := time.Now().UTC().Truncate(time.Millisecond)
	window := 10 * time.Minute

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `
DELETE FROM bnpl.checkout_otp_rate_limits
WHERE scope_key = ANY($1)`,
			[]string{phoneKey, merchantKey},
		)
	})

	// Pre-fill the merchant bucket to capacity so the second consume rejects.
	_, err := pool.Exec(ctx, `
INSERT INTO bnpl.checkout_otp_rate_limits (
	scope, scope_key, window_started_at, send_count, updated_at
) VALUES ($1, $2, $3, 1, $3)`,
		model.CheckoutOTPRateLimitScopeMerchant, merchantKey, now,
	)
	require.NoError(t, err)

	conn, err := pool.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()

	_, err = conn.Exec(ctx, "SET search_path TO bnpl")
	require.NoError(t, err)

	adapterConn := &postgres.Conn{Conn: conn}
	txErr := adapterConn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
		pgTx := postgres.MustTx(tx)

		if err := checkoutsessionrp.ConsumeOTPRateLimit(
			ctx,
			pgTx,
			model.CheckoutOTPRateLimitScopePhone,
			phoneKey,
			window,
			5,
			now,
		); err != nil {
			return err
		}

		err := checkoutsessionrp.ConsumeOTPRateLimit(
			ctx,
			pgTx,
			model.CheckoutOTPRateLimitScopeMerchant,
			merchantKey,
			window,
			1,
			now,
		)
		if cerr.IsCode(err, cerr.CodeTooManyRequests) {
			// Mirror issueOTP: cross-checkout rejection aborts the TX.
			return err
		}

		return err
	})
	require.True(t, cerr.IsCode(txErr, cerr.CodeTooManyRequests), "got %v", txErr)

	var phoneCount int
	err = pool.QueryRow(ctx, `
SELECT COALESCE(send_count, 0)
FROM bnpl.checkout_otp_rate_limits
WHERE scope = $1 AND scope_key = $2`,
		model.CheckoutOTPRateLimitScopePhone, phoneKey,
	).Scan(&phoneCount)
	require.Error(t, err, "phone bucket row must not exist after rollback")
	require.Contains(t, err.Error(), "no rows")

	var merchantCount int
	err = pool.QueryRow(ctx, `
SELECT send_count
FROM bnpl.checkout_otp_rate_limits
WHERE scope = $1 AND scope_key = $2`,
		model.CheckoutOTPRateLimitScopeMerchant, merchantKey,
	).Scan(&merchantCount)
	require.NoError(t, err)
	require.Equal(t, 1, merchantCount)
}

// runConcurrentConsume runs one ConsumeOTPRateLimit worker. Setup failures are
// returned via the result channel so require.* runs only on the main test
// goroutine (avoiding FailNow deadlocks against barriers).
func runConcurrentConsume(
	ctx context.Context,
	pool *pgxpool.Pool,
	ready chan<- struct{},
	start <-chan struct{},
	scope model.CheckoutOTPRateLimitScope,
	scopeKey string,
	window time.Duration,
	maxSends int,
	now time.Time,
) concurrentConsumeResult {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		ready <- struct{}{}
		<-start

		return concurrentConsumeResult{err: fmt.Errorf("acquire: %w", err)}
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SET search_path TO bnpl"); err != nil {
		ready <- struct{}{}
		<-start

		return concurrentConsumeResult{err: fmt.Errorf("set search_path: %w", err)}
	}

	adapterConn := &postgres.Conn{Conn: conn}
	consumeErr := adapterConn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
		pgTx := postgres.MustTx(tx)
		ready <- struct{}{}
		<-start

		return checkoutsessionrp.ConsumeOTPRateLimit(
			ctx,
			pgTx,
			scope,
			scopeKey,
			window,
			maxSends,
			now,
		)
	})

	return concurrentConsumeResult{err: consumeErr}
}

func openIntegrationPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	dsn := postgresDSNFromEnv()
	if dsn == "" {
		t.Skip("POSTGRES_* env not configured; skipping PostgreSQL OTP rate-limit integration test")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres unavailable: %v", err)
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
		user, password, host, port, db, sslMode,
	)
}
