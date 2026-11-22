//go:build development && integration

package contractuc

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/checkoutsessionrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/creditaccountrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/customerprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/outboxrp"
	"github.com/parstis/bnpl-backend/pkg/core/event/notificationsms"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func TestCreditDebitedNotification_OutboxAtomicWithExposure(t *testing.T) {
	dsn := os.Getenv("COLLECTIONS_TEST_DATABASE_URL")
	require.NotEmpty(t, dsn, "dedicated disposable PostgreSQL is required; tests never skip")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)

	schema := "credit_debit_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := postgres.NewPool(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	_, err = pool.Pool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)

	ddl, err := os.ReadFile("../../../adapter/db/postgres/migrationsrp/000_schema.sql")
	require.NoError(t, err)
	baseline := strings.SplitN(string(ddl), "-- MANAGED DATABASE PERMISSIONS: canonical initial grants (pre-release)", 2)[0]
	baseline = rewriteActivationFixtureSchemaSQL(baseline, schema)
	_, err = pool.Pool.Exec(ctx, baseline)
	require.NoError(t, err)

	customerID := uuid.New()
	externalCustomerID := uuid.New()
	merchantID := uuid.New()
	externalMerchantID := uuid.New()
	branchID := uuid.New()
	accountID := uuid.New()
	decisionID := uuid.New()
	contractID := uuid.New()
	sessionID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		_, execErr := pool.Pool.Exec(ctx, sql, args...)
		require.NoError(t, execErr)
	}

	mustExec(
		`INSERT INTO customer_profiles(id,external_subject_id,external_customer_id,jurisdiction,status)
VALUES($1,$2,$3,'IR','active')`,
		customerID, uuid.New(), externalCustomerID,
	)
	mustExec(
		`INSERT INTO merchant_profiles(id,external_merchant_id,display_name,status,risk_tier,max_tenure_months,jurisdiction)
VALUES($1,$2,'test','active','low',12,'IR')`,
		merchantID, externalMerchantID,
	)
	mustExec(
		`INSERT INTO merchant_branch_profiles(
id,external_merchant_id,external_branch_id,branch_type,status,jurisdiction,offer_preparation_mode
) VALUES($1,$2,$3,'online','active','IR','on_start')`,
		branchID, externalMerchantID, uuid.New(),
	)
	mustExec(
		`INSERT INTO decisions(
id,type,target_type,target_id,outcome,reason_codes,policy_id,policy_version,jurisdiction,actor_type,decided_at,idempotency_key,idempotency_fingerprint
) VALUES($1,'credit_limit','customer_profile',$2,'approved',ARRAY['CHECKOUT_APPROVED'],'checkout','v1','IR','system',$3,$4,$5)`,
		decisionID, customerID, now, uuid.NewString(), "sha256:"+strings.Repeat("1", 64),
	)
	mustExec(
		`INSERT INTO credit_accounts(
id,customer_id,currency,credit_kind,limit_amount_minor,exposure_amount_minor,status,effective_from
) VALUES($1,$2,'IRR','pay_next_month',5000000,0,'active',$3)`,
		accountID, customerID, now,
	)
	mustExec(
		`INSERT INTO contracts(
id,decision_id,checkout_key,customer_id,credit_account_id,merchant_profile_id,currency,
requested_amount_minor,approved_financed_amount_minor,installment_count,payable_now_amount_minor,
remaining_principal_amount_minor,status,expires_at,activated_at
) VALUES($1,$2,$3,$4,$5,$6,'IRR',100,100,1,0,100,'active',$7,$8)`,
		contractID, decisionID, uuid.NewString(), customerID, accountID, merchantID, now.Add(time.Hour), now,
	)
	publicToken := "cs_" + strings.Repeat("a", 64)
	verifyDeadline := now.Add(30 * time.Minute)
	mustExec(
		`INSERT INTO checkout_sessions(
id,public_token,origin,merchant_profile_id,merchant_branch_profile_id,external_online_store_id,
idempotency_key,merchant_order_id,offer_preparation_mode,customer_id,return_destination,return_url,
requested_amount_minor,currency,status,decision_id,contract_id,expires_at,credit_kind,
verification_pending_at,verification_deadline_at,verification_policy_version
) VALUES(
$1,$2,'internal',$3,$4,$5,$6,$7,'on_start',$8,'default','https://merchant.example/return',
100,'IRR','verify_pending',$9,$10,$11,'pay_next_month',$12,$13,'v1'
)`,
		sessionID, publicToken, merchantID, branchID, uuid.New(), uuid.NewString(), uuid.NewString(),
		customerID, decisionID, contractID, now.Add(time.Hour), now, verifyDeadline,
	)

	uc := &UseCase{
		customerProfile:                  customerprofilerp.New(),
		creditAccount:                    creditaccountrp.New(),
		checkoutSession:                  checkoutsessionrp.New(),
		outbox:                           outboxrp.New(),
		notificationCommandsTopic:        "notification.commands.v1",
		creditDebitedNotificationEnabled: true,
	}

	debitAmount := uint64(500_000)
	var creditTx model.CreditTransaction
	var updatedAccount model.CreditAccount

	err = pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			creditAccountQ := uc.creditAccount.Tx(tx)
			var addErr error
			updatedAccount, addErr = creditAccountQ.AddExposure(ctx, model.CreditAccountID(accountID), debitAmount, now)
			if addErr != nil {
				return addErr
			}

			var appendErr error
			creditTx, appendErr = creditAccountQ.AppendTransaction(
				ctx,
				model.CreditAccountID(accountID),
				model.ExposureIncreasedCreditTransactionType,
				debitAmount,
				model.IRRCurrency,
				model.ContractActivatedCreditTransactionReason,
				model.ContractCreditTransactionReferenceType,
				contractID,
				now,
				now,
			)
			if appendErr != nil {
				return appendErr
			}

			return uc.enqueueCreditDebitedNotification(
				ctx,
				tx,
				model.Contract{
					ID:         model.ContractID(contractID),
					CustomerID: model.CustomerID(customerID),
				},
				updatedAccount,
				creditTx,
				debitAmount,
				now,
			)
		})
	})
	require.NoError(t, err)

	var outboxCount int
	require.NoError(t, pool.Pool.QueryRow(ctx, `
SELECT count(*) FROM integration_outbox
WHERE event_type=$1 AND destination=$2
`, notificationsms.EventTypeCommand, "notification.commands.v1").Scan(&outboxCount))
	require.Equal(t, 1, outboxCount)

	var payload []byte
	require.NoError(t, pool.Pool.QueryRow(ctx, `
SELECT payload FROM integration_outbox
WHERE event_type=$1
`, notificationsms.EventTypeCommand).Scan(&payload))

	var event notificationsms.Command
	require.NoError(t, json.Unmarshal(payload, &event))
	require.NoError(t, notificationsms.ValidateCreditDebitedCommand(event))
	require.Equal(t, creditTx.ID.String(), event.SourceEventID)
	require.Equal(t, notificationsms.CreditDebitedIdempotencyKey(creditTx.ID.String()), event.IdempotencyKey)
	require.Equal(t, notificationsms.NotificationTypeCreditMonthlyDebited, event.NotificationType)
	require.Equal(t, externalCustomerID.String(), event.Targets[0].ExternalCustomerID)
	require.Equal(t, "cs_"+sessionID.String(), event.Targets[0].BusinessReferenceID)
	require.Equal(t, "500000", event.Targets[0].Deliveries[0].Variables["debited_amount"])
	require.Equal(t, "4500000", event.Targets[0].Deliveries[0].Variables["remaining_available_credit"])
	require.NotContains(t, string(payload), "x-notification-caller")
	require.NotContains(t, string(payload), "payment_url")

	// Rollback path: enqueue in a TX that rolls back must leave no additional row.
	err = pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			fakeTx := model.CreditTransaction{ID: model.CreditTransactionID(uuid.New()), AmountMinor: 1}
			enqueueErr := uc.enqueueCreditDebitedNotification(
				ctx,
				tx,
				model.Contract{
					ID:         model.ContractID(contractID),
					CustomerID: model.CustomerID(customerID),
				},
				model.CreditAccount{
					CreditKind:          model.PayNextMonthCreditKind,
					Currency:            model.IRRCurrency,
					LimitAmountMinor:    100,
					ExposureAmountMinor: 1,
				},
				fakeTx,
				1,
				now,
			)
			require.NoError(t, enqueueErr)
			return context.Canceled
		})
	})
	require.Error(t, err)

	require.NoError(t, pool.Pool.QueryRow(ctx, `
SELECT count(*) FROM integration_outbox
WHERE event_type=$1
`, notificationsms.EventTypeCommand).Scan(&outboxCount))
	require.Equal(t, 1, outboxCount)

	// Feature flag off must not create a command.
	uc.creditDebitedNotificationEnabled = false
	err = pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			return uc.enqueueCreditDebitedNotification(
				ctx,
				tx,
				model.Contract{
					ID:         model.ContractID(contractID),
					CustomerID: model.CustomerID(customerID),
				},
				updatedAccount,
				model.CreditTransaction{ID: model.CreditTransactionID(uuid.New()), AmountMinor: 10},
				10,
				now,
			)
		})
	})
	require.NoError(t, err)
	require.NoError(t, pool.Pool.QueryRow(ctx, `
SELECT count(*) FROM integration_outbox
WHERE event_type=$1
`, notificationsms.EventTypeCommand).Scan(&outboxCount))
	require.Equal(t, 1, outboxCount)
}
