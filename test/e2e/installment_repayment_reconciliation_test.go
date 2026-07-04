//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
)

// TestE2E_InstallmentRepayment_LostFCResponse_StaleCreatedReconcilesWithoutDuplicate
// drives logical time with the harness Fixed clock / aged created_at — it does not
// sleep for PaymentSessionTTL or created_timeout.
func TestE2E_InstallmentRepayment_LostFCResponse_StaleCreatedReconcilesWithoutDuplicate(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	require.Equal(t, model.CompletedCheckoutSessionStatus, out.Session.Status)

	contractRow := h.queryContractBySession(out.Session)
	require.Equal(t, model.ActiveContractStatus, contractRow.Status)
	require.NotNil(t, contractRow.FinancialMerchantEntitlementID)

	installmentID, amountMinor := queryNextUnpaidInstallment(t, contractRow.ID)
	require.False(t, installmentID.IsZero())

	root, err := repositoryRoot()
	require.NoError(t, err)
	cfg, err := config.Load(filepath.Join(root, "configs/config.yaml"))
	require.NoError(t, err)

	pool := &postgres.Pool{Pool: testDB}
	idGen := e2eUUIDIdentifierGenerator{}
	repaymentUC, err := cfg.NewInstallmentRepaymentUseCase(
		pool,
		h.repos.CustomerProfile,
		h.repos.MerchantProfile,
		h.repos.Contract,
		h.repos.Installment,
		h.repos.InstallmentCharge,
		h.repos.PaymentAttempt,
		h.repos.Outbox,
		h.fc,
		h.clock,
		h.logger,
		idGen,
	)
	require.NoError(t, err)

	h.fc.setPaymentLostAfterCommit()

	tenantID := model.TenantID(uuid.MustParse("00000000-0000-4000-8000-0000000000aa"))
	contextID := model.ContextID(uuid.MustParse("00000000-0000-4000-8000-0000000000bb"))
	idempotencyKey := "e2e-installment-idem-" + uuid.NewString()

	createOut, err := repaymentUC.CreatePaymentAttempt(h.ctx, installmentrepaymentuc.CreatePaymentAttemptInput{
		TenantID:           tenantID,
		ContextID:          contextID,
		ContractID:         contractRow.ID,
		InstallmentID:      installmentID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     idempotencyKey,
		Channel:            model.GatewayPaymentChannel,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_ = createOut

	attempt := queryInstallmentRepaymentAttemptByIdempotency(t, installmentID, idempotencyKey)
	require.Equal(t, model.CreatedPaymentAttemptState, attempt.State)
	require.Equal(t, amountMinor, attempt.AmountMinor)
	require.NotNil(t, attempt.FinancialCollectionObligationID)
	require.Nil(t, attempt.FinancialPaymentIntentID)
	require.Equal(t, 1, h.fc.committedPaymentCount())

	// Age the durable created_at past created_timeout without wall-clock waiting.
	createdTimeout := cfg.Usecases.PaymentAttemptReconciliation.CreatedTimeout
	require.Greater(t, createdTimeout, time.Duration(0))
	staleCreatedAt := h.clock.Now().UTC().Add(-createdTimeout - time.Minute)
	updatePaymentAttemptCreatedAt(t, attempt.ID, staleCreatedAt)

	// Crossing the requested session TTL must not invent a replacement operation.
	h.advanceClock(installmentrepaymentuc.DefaultPaymentSessionTTL + time.Minute)
	_, err = repaymentUC.CreatePaymentAttempt(h.ctx, installmentrepaymentuc.CreatePaymentAttemptInput{
		TenantID:           tenantID,
		ContextID:          contextID,
		ContractID:         contractRow.ID,
		InstallmentID:      installmentID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     idempotencyKey,
		Channel:            model.GatewayPaymentChannel,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
	require.Equal(t, 1, h.fc.committedPaymentCount())

	afterTTL := queryInstallmentRepaymentAttemptByIdempotency(t, installmentID, idempotencyKey)
	require.Equal(t, model.CreatedPaymentAttemptState, afterTTL.State)
	require.Equal(t, attempt.ID, afterTTL.ID)
	require.Equal(t, idempotencyKey, afterTTL.IdempotencyKey)

	reconciliationUC, err := cfg.NewPaymentAttemptReconciliationUseCase(
		pool,
		h.repos.PaymentAttempt,
		h.repos.Contract,
		h.repos.Outbox,
		idGen,
		h.clock,
	)
	require.NoError(t, err)

	require.NoError(t, reconciliationUC.ProcessDue(h.ctx))

	reconciled := queryInstallmentRepaymentAttemptByIdempotency(t, installmentID, idempotencyKey)
	require.Equal(t, model.ReconciliationRequiredPaymentAttemptState, reconciled.State)
	require.NotNil(t, reconciled.ReconciliationReason)
	require.Equal(t, model.PaymentSessionCreationTimeoutPaymentReconciliationReason, *reconciled.ReconciliationReason)
	require.Nil(t, reconciled.FinancialPaymentIntentID)
	require.Equal(t, *attempt.FinancialCollectionObligationID, *reconciled.FinancialCollectionObligationID)
	require.Equal(t, 1, h.fc.committedPaymentCount())
	require.Equal(t, 1, countInstallmentRepaymentAttempts(t, contractRow.ID))

	payload := latestOutboxPayload(t, installmentpaymentreconciliation.ReconciliationRequestedEventType, attempt.ID.String())
	var event installmentpaymentreconciliation.RequestEvent
	require.NoError(t, json.Unmarshal(payload, &event))
	require.Equal(t, attempt.ID, event.Data.BNPLPaymentAttemptID)
	require.Equal(t, contractRow.ID, event.Data.ContractID)
	require.Equal(t, installmentID, event.Data.InstallmentID)
	require.Nil(t, event.Data.FinancialPaymentIntentID)
	require.Equal(t, model.PaymentSessionCreationTimeoutPaymentReconciliationReason, event.Data.ReconciliationReason)
}

type e2eUUIDIdentifierGenerator struct{}

func (e2eUUIDIdentifierGenerator) Generate() string {
	return uuid.NewString()
}

func queryNextUnpaidInstallment(t *testing.T, contractID model.ContractID) (model.InstallmentID, uint64) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var (
		id          uuid.UUID
		amountMinor uint64
	)
	err := testDB.QueryRow(ctx, `
		SELECT id, principal_amount_minor
		FROM bnpl.installments
		WHERE contract_id = $1
		  AND status IN ('scheduled', 'due', 'overdue')
		ORDER BY installment_order ASC
		LIMIT 1
	`, contractID).Scan(&id, &amountMinor)
	require.NoError(t, err)

	return model.InstallmentID(id), amountMinor
}

func queryInstallmentRepaymentAttemptByIdempotency(
	t *testing.T,
	installmentID model.InstallmentID,
	idempotencyKey string,
) model.PaymentAttempt {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var attempt model.PaymentAttempt
	err := testDB.QueryRow(ctx, `
		SELECT
			id, contract_id, installment_id, amount_minor, currency, purpose, state,
			idempotency_key, financial_collection_obligation_id, financial_payment_intent_id,
			reconciliation_reason, created_at
		FROM bnpl.payment_attempts
		WHERE installment_id = $1
		  AND idempotency_key = $2
		  AND purpose = $3
	`, installmentID, idempotencyKey, model.InstallmentRepaymentFinancialCorePaymentPurpose).Scan(
		&attempt.ID,
		&attempt.ContractID,
		&attempt.InstallmentID,
		&attempt.AmountMinor,
		&attempt.Currency,
		&attempt.Purpose,
		&attempt.State,
		&attempt.IdempotencyKey,
		&attempt.FinancialCollectionObligationID,
		&attempt.FinancialPaymentIntentID,
		&attempt.ReconciliationReason,
		&attempt.CreatedAt,
	)
	require.NoError(t, err)

	return attempt
}

func updatePaymentAttemptCreatedAt(t *testing.T, id model.PaymentAttemptID, createdAt time.Time) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := testDB.Exec(ctx, `
		UPDATE bnpl.payment_attempts
		SET created_at = $2, updated_at = $2
		WHERE id = $1
	`, id, createdAt.UTC())
	require.NoError(t, err)
}

func countInstallmentRepaymentAttempts(t *testing.T, contractID model.ContractID) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := testDB.QueryRow(ctx, `
		SELECT count(*)
		FROM bnpl.payment_attempts
		WHERE contract_id = $1
		  AND purpose = $2
	`, contractID, model.InstallmentRepaymentFinancialCorePaymentPurpose).Scan(&count)
	require.NoError(t, err)

	return count
}

func latestOutboxPayload(t *testing.T, eventType, messageKey string) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var payload []byte
	err := testDB.QueryRow(ctx, `
		SELECT payload
		FROM bnpl.integration_outbox
		WHERE event_type = $1
		  AND message_key = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, eventType, messageKey).Scan(&payload)
	require.NoError(t, err)

	return payload
}
