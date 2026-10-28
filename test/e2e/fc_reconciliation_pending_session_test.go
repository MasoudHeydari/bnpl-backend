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
	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentreconciliationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
)

func seedInstallmentRepaymentReconciliationRequired(
	t *testing.T,
) (*confirmSagaHarness, model.PaymentAttempt, *config.Config) {
	t.Helper()

	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)

	contractRow := h.queryContractBySession(out.Session)
	installmentID, amountMinor := queryNextUnpaidInstallment(t, contractRow.ID)

	root, err := repositoryRoot()
	require.NoError(t, err)
	cfg, err := config.Load(filepath.Join(root, "configs/runtime.yaml"))
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
	idempotencyKey := "e2e-recon-pending-" + uuid.NewString()

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

	attempt := queryInstallmentRepaymentAttemptByIdempotency(t, installmentID, idempotencyKey)
	require.Equal(t, amountMinor, attempt.AmountMinor)

	createdTimeout := cfg.Usecases.PaymentAttemptReconciliation.CreatedTimeout
	updatePaymentAttemptCreatedAt(t, attempt.ID, h.clock.Now().UTC().Add(-createdTimeout-time.Minute))

	paymentAttemptReconciliationUC, err := cfg.NewPaymentAttemptReconciliationUseCase(
		pool,
		h.repos.PaymentAttempt,
		h.repos.Contract,
		h.repos.Outbox,
		idGen,
		h.clock,
	)
	require.NoError(t, err)
	require.NoError(t, paymentAttemptReconciliationUC.ProcessDue(h.ctx))

	reconciled := queryInstallmentRepaymentAttemptFull(t, installmentID, idempotencyKey)
	require.Equal(t, model.ReconciliationRequiredPaymentAttemptState, reconciled.State)
	require.Equal(t, 1, reconciled.ReconciliationAttemptCount)
	require.Equal(t, 1, h.fc.committedPaymentCount())

	return h, reconciled, cfg
}

func newFinancialCoreReconciliationUC(
	t *testing.T,
	h *confirmSagaHarness,
	cfg *config.Config,
) *financialcorepaymentreconciliationuc.UseCase {
	t.Helper()

	pool := &postgres.Pool{Pool: testDB}
	uc, err := cfg.NewFinancialCorePaymentReconciliationUseCase(
		pool,
		h.PaymentResult,
		h.repos.Inbox,
		h.repos.Outbox,
		h.repos.PaymentAttempt,
		h.repos.Contract,
		h.repos.Installment,
		h.repos.InstallmentCharge,
		h.repos.CreditAccount,
		h.fingerprintHasher,
		e2eUUIDIdentifierGenerator{},
		h.clock,
		h.logger,
	)
	require.NoError(t, err)

	return uc
}

func pendingReconciliationInput(
	attempt model.PaymentAttempt,
	eventID string,
	url *string,
	expiresAt *time.Time,
	amountMinor uint64,
	intentID string,
	sessionID string,
	now time.Time,
) financialcorepaymentreconciliationuc.ProcessInput {
	data := installmentpaymentreconciliation.ResultEventData{
		RequestEventID:           installmentpaymentreconciliation.RequestEventID(attempt.ID, attempt.ReconciliationAttemptCount),
		BNPLPaymentAttemptID:     attempt.ID,
		ReconciliationGeneration: attempt.ReconciliationAttemptCount,
		Status:                   installmentpaymentreconciliation.PendingResultStatus,
		AmountMinor:              amountMinor,
		Currency:                 string(attempt.Currency),
		OccurredAt:               now,
	}
	if intentID != "" {
		data.FinancialPaymentIntentID = &intentID
	}

	if sessionID != "" {
		data.FinancialPaymentSessionID = &sessionID
	}

	data.PaymentURL = url
	data.ExpiresAt = expiresAt

	raw, _ := json.Marshal(map[string]any{
		"event_id": eventID,
		"data":     data,
	})

	return financialcorepaymentreconciliationuc.ProcessInput{
		EventID:       eventID,
		EventType:     installmentpaymentreconciliation.ReconciliationResultEventType,
		EventVersion:  installmentpaymentreconciliation.ReconciliationResultEventVersion,
		SourceService: installmentpaymentreconciliation.FinancialCoreSourceService,
		RawPayload:    raw,
		Data:          data,
	}
}

func queryInstallmentRepaymentAttemptFull(
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
			financial_payment_session_id, payment_url, financial_payment_expires_at,
			reconciliation_reason, reconciliation_attempt_count, created_at
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
		&attempt.FinancialPaymentSessionID,
		&attempt.PaymentURL,
		&attempt.FinancialPaymentExpiresAt,
		&attempt.ReconciliationReason,
		&attempt.ReconciliationAttemptCount,
		&attempt.CreatedAt,
	)
	require.NoError(t, err)

	return attempt
}

func queryInboxStatus(t *testing.T, eventID string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var status string
	err := testDB.QueryRow(ctx, `
		SELECT status
		FROM bnpl.integration_inbox
		WHERE event_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, eventID).Scan(&status)
	require.NoError(t, err)

	return status
}

func TestE2E_ReconciliationPending_CompleteSessionTransitionsToSubmitted(t *testing.T) {
	h, attempt, cfg := seedInstallmentRepaymentReconciliationRequired(t)
	uc := newFinancialCoreReconciliationUC(t, h, cfg)

	now := h.clock.Now().UTC()
	url := "https://pay.example/recon/A"
	expires := now.Add(20 * time.Minute)
	eventID := "evt-recon-complete-" + uuid.NewString()

	out, err := uc.Process(h.ctx, pendingReconciliationInput(
		attempt,
		eventID,
		&url,
		&expires,
		attempt.AmountMinor,
		"pi-recon-1",
		"ps-recon-1",
		now,
	))
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentreconciliationuc.ProcessResultResolvedSubmitted, out.Result)

	stored := queryInstallmentRepaymentAttemptFull(t, attempt.InstallmentID, attempt.IdempotencyKey)
	require.Equal(t, model.SubmittedPaymentAttemptState, stored.State)
	require.NotNil(t, stored.FinancialPaymentIntentID)
	require.Equal(t, "pi-recon-1", *stored.FinancialPaymentIntentID)
	require.NotNil(t, stored.FinancialPaymentSessionID)
	require.Equal(t, "ps-recon-1", *stored.FinancialPaymentSessionID)
	require.NotNil(t, stored.PaymentURL)
	require.Equal(t, url, *stored.PaymentURL)
	require.NotNil(t, stored.FinancialPaymentExpiresAt)
	require.True(t, stored.FinancialPaymentExpiresAt.UTC().Equal(expires.UTC()))
	require.Equal(t, "processed", queryInboxStatus(t, eventID))
	require.Equal(t, 1, h.fc.committedPaymentCount())
	require.Equal(t, 1, countInstallmentRepaymentAttempts(t, attempt.ContractID))
}

func TestE2E_ReconciliationPending_ResolvesOriginalFakeFCIdentities(t *testing.T) {
	h, attempt, cfg := seedInstallmentRepaymentReconciliationRequired(t)
	uc := newFinancialCoreReconciliationUC(t, h, cfg)

	committed, ok := h.fc.committedPaymentByKey(attempt.IdempotencyKey)
	require.True(t, ok, "Fake FC must retain the originally committed CreateCheckoutPayment")
	require.NotEmpty(t, committed.PaymentIntentID)
	require.NotEmpty(t, committed.PaymentSessionID)
	require.NotEmpty(t, committed.PaymentLink)
	require.False(t, committed.ExpiresAt.IsZero())

	now := h.clock.Now().UTC()
	eventID := "evt-recon-real-fc-" + uuid.NewString()
	url := committed.PaymentLink
	expires := committed.ExpiresAt.UTC()

	out, err := uc.Process(h.ctx, pendingReconciliationInput(
		attempt,
		eventID,
		&url,
		&expires,
		attempt.AmountMinor,
		committed.PaymentIntentID,
		committed.PaymentSessionID,
		now,
	))
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentreconciliationuc.ProcessResultResolvedSubmitted, out.Result)

	stored := queryInstallmentRepaymentAttemptFull(t, attempt.InstallmentID, attempt.IdempotencyKey)
	require.Equal(t, attempt.ID, stored.ID)
	require.Equal(t, attempt.IdempotencyKey, stored.IdempotencyKey)
	require.Equal(t, model.SubmittedPaymentAttemptState, stored.State)
	require.NotNil(t, stored.FinancialPaymentIntentID)
	require.Equal(t, committed.PaymentIntentID, *stored.FinancialPaymentIntentID)
	require.NotNil(t, stored.FinancialPaymentSessionID)
	require.Equal(t, committed.PaymentSessionID, *stored.FinancialPaymentSessionID)
	require.NotNil(t, stored.PaymentURL)
	require.Equal(t, committed.PaymentLink, *stored.PaymentURL)
	require.NotNil(t, stored.FinancialPaymentExpiresAt)
	require.True(t, stored.FinancialPaymentExpiresAt.UTC().Equal(committed.ExpiresAt.UTC()))
	require.Equal(t, "processed", queryInboxStatus(t, eventID))
	require.Equal(t, 1, h.fc.committedPaymentCount())
	require.Equal(t, 1, countInstallmentRepaymentAttempts(t, attempt.ContractID))
	require.Equal(t, 1, len(h.fc.committedPaymentKeys()))
}

func TestE2E_ReconciliationPending_IncompleteSessionRemainsReconcilable(t *testing.T) {
	h, attempt, cfg := seedInstallmentRepaymentReconciliationRequired(t)
	uc := newFinancialCoreReconciliationUC(t, h, cfg)

	now := h.clock.Now().UTC()
	eventID := "evt-recon-incomplete-" + uuid.NewString()

	out, err := uc.Process(h.ctx, pendingReconciliationInput(
		attempt,
		eventID,
		nil,
		nil,
		attempt.AmountMinor,
		"pi-recon-incomplete",
		"ps-recon-incomplete",
		now,
	))
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentreconciliationuc.ProcessResultStillReconciliationRequired, out.Result)

	stored := queryInstallmentRepaymentAttemptFull(t, attempt.InstallmentID, attempt.IdempotencyKey)
	require.Equal(t, model.ReconciliationRequiredPaymentAttemptState, stored.State)
	require.Nil(t, stored.PaymentURL)
	require.Nil(t, stored.FinancialPaymentExpiresAt)
	require.Equal(t, "processed", queryInboxStatus(t, eventID))
	require.Equal(t, 1, h.fc.committedPaymentCount())
}

func TestE2E_ReconciliationPending_AmountMismatchPermanentlyRejected(t *testing.T) {
	h, attempt, cfg := seedInstallmentRepaymentReconciliationRequired(t)
	uc := newFinancialCoreReconciliationUC(t, h, cfg)

	now := h.clock.Now().UTC()
	url := "https://pay.example/recon/conflict"
	expires := now.Add(20 * time.Minute)
	eventID := "evt-recon-amount-" + uuid.NewString()

	out, err := uc.Process(h.ctx, pendingReconciliationInput(
		attempt,
		eventID,
		&url,
		&expires,
		attempt.AmountMinor+1,
		"pi-recon-amount",
		"ps-recon-amount",
		now,
	))
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentreconciliationuc.ProcessResultPermanentlyRejected, out.Result)

	stored := queryInstallmentRepaymentAttemptFull(t, attempt.InstallmentID, attempt.IdempotencyKey)
	require.Equal(t, model.ReconciliationRequiredPaymentAttemptState, stored.State)
	require.Nil(t, stored.PaymentURL)
	require.Equal(t, "failed", queryInboxStatus(t, eventID))
	require.Equal(t, 1, h.fc.committedPaymentCount())
}

func TestE2E_ReconciliationPending_LaterThanRequestedExpiryAccepted(t *testing.T) {
	h, attempt, cfg := seedInstallmentRepaymentReconciliationRequired(t)
	uc := newFinancialCoreReconciliationUC(t, h, cfg)

	now := h.clock.Now().UTC()
	url := "https://pay.example/recon/late"
	requested := attempt.CreatedAt.UTC().Add(installmentrepaymentuc.DefaultPaymentSessionTTL)
	expires := requested.Add(15 * time.Minute)
	eventID := "evt-recon-late-" + uuid.NewString()

	out, err := uc.Process(h.ctx, pendingReconciliationInput(
		attempt,
		eventID,
		&url,
		&expires,
		attempt.AmountMinor,
		"pi-recon-late",
		"ps-recon-late",
		now,
	))
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentreconciliationuc.ProcessResultResolvedSubmitted, out.Result)

	stored := queryInstallmentRepaymentAttemptFull(t, attempt.InstallmentID, attempt.IdempotencyKey)
	require.Equal(t, model.SubmittedPaymentAttemptState, stored.State)
	require.NotNil(t, stored.FinancialPaymentExpiresAt)
	require.True(t, stored.FinancialPaymentExpiresAt.UTC().Equal(expires.UTC()))
	require.True(t, stored.FinancialPaymentExpiresAt.UTC().After(requested))
	require.Equal(t, 1, h.fc.committedPaymentCount())
}

func TestE2E_ReconciliationPending_InboxFingerprintPayloadMismatch(t *testing.T) {
	h, attempt, cfg := seedInstallmentRepaymentReconciliationRequired(t)
	uc := newFinancialCoreReconciliationUC(t, h, cfg)

	now := h.clock.Now().UTC()
	urlA := "https://pay.example/recon/A"
	urlB := "https://pay.example/recon/B"
	expires := now.Add(20 * time.Minute)
	eventID := "evt-recon-fp-" + uuid.NewString()

	first, err := uc.Process(h.ctx, pendingReconciliationInput(
		attempt,
		eventID,
		&urlA,
		&expires,
		attempt.AmountMinor,
		"pi-recon-fp",
		"ps-recon-fp",
		now,
	))
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentreconciliationuc.ProcessResultResolvedSubmitted, first.Result)

	second, err := uc.Process(h.ctx, pendingReconciliationInput(
		attempt,
		eventID,
		&urlB,
		&expires,
		attempt.AmountMinor,
		"pi-recon-fp",
		"ps-recon-fp",
		now,
	))
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentreconciliationuc.ProcessResultPayloadConflictRecorded, second.Result)

	stored := queryInstallmentRepaymentAttemptFull(t, attempt.InstallmentID, attempt.IdempotencyKey)
	require.Equal(t, model.SubmittedPaymentAttemptState, stored.State)
	require.NotNil(t, stored.PaymentURL)
	require.Equal(t, urlA, *stored.PaymentURL)
	require.Equal(t, 1, h.fc.committedPaymentCount())
}

func TestE2E_InstallmentCreate_FCExpiryLaterThanRequestedAccepted(t *testing.T) {
	resetDatabase(t)

	externalCustomerID := newExternalCustomer(t)
	h := newConfirmSagaHarness(t, confirmSagaHarnessOptions{externalCustomerID: externalCustomerID})
	seed := seedMerchantFundingCheckout(t, h, externalCustomerID)

	out, err := h.CheckoutSession.Confirm(h.ctx, confirmInputForSeed(seed))
	require.NoError(t, err)
	contractRow := h.queryContractBySession(out.Session)
	installmentID, _ := queryNextUnpaidInstallment(t, contractRow.ID)

	root, err := repositoryRoot()
	require.NoError(t, err)
	cfg, err := config.Load(filepath.Join(root, "configs/runtime.yaml"))
	require.NoError(t, err)

	pool := &postgres.Pool{Pool: testDB}
	h.fc.setPaymentExpiresAtOffset(15 * time.Minute)

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
		e2eUUIDIdentifierGenerator{},
	)
	require.NoError(t, err)

	idempotencyKey := "e2e-late-expiry-" + uuid.NewString()
	createOut, err := repaymentUC.CreatePaymentAttempt(h.ctx, installmentrepaymentuc.CreatePaymentAttemptInput{
		TenantID:           model.TenantID(uuid.MustParse("00000000-0000-4000-8000-0000000000aa")),
		ContextID:          model.ContextID(uuid.MustParse("00000000-0000-4000-8000-0000000000bb")),
		ContractID:         contractRow.ID,
		InstallmentID:      installmentID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     idempotencyKey,
		Channel:            model.GatewayPaymentChannel,
	})
	require.NoError(t, err)
	require.Equal(t, model.SubmittedPaymentAttemptState, createOut.PaymentAttempt.State)
	require.NotNil(t, createOut.PaymentAttempt.FinancialPaymentExpiresAt)

	requested := createOut.PaymentAttempt.CreatedAt.UTC().Add(installmentrepaymentuc.DefaultPaymentSessionTTL)
	require.True(t, createOut.PaymentAttempt.FinancialPaymentExpiresAt.UTC().Equal(requested.Add(15*time.Minute)))
	require.Equal(t, 1, h.fc.committedPaymentCount())

	// Exact replay must not create a second FC operation.
	replay, err := repaymentUC.CreatePaymentAttempt(h.ctx, installmentrepaymentuc.CreatePaymentAttemptInput{
		TenantID:           model.TenantID(uuid.MustParse("00000000-0000-4000-8000-0000000000aa")),
		ContextID:          model.ContextID(uuid.MustParse("00000000-0000-4000-8000-0000000000bb")),
		ContractID:         contractRow.ID,
		InstallmentID:      installmentID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     idempotencyKey,
		Channel:            model.GatewayPaymentChannel,
	})
	require.NoError(t, err)
	require.Equal(t, createOut.PaymentAttempt.ID, replay.PaymentAttempt.ID)
	require.Equal(t, 1, h.fc.committedPaymentCount())
}
