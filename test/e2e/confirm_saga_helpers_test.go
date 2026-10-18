//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

func (h *confirmSagaHarness) assertAttemptMatchesSaga(session model.CheckoutSession) {
	h.t.Helper()

	require.NotNil(h.t, session.ConfirmationAttemptID)
	require.False(h.t, session.ConfirmationAttemptID.IsZero())

	instance, err := h.loadConfirmationSagaByAttempt(*session.ConfirmationAttemptID)
	require.NoError(h.t, err)
	require.Equal(h.t, model.SagaBusinessKey(session.ConfirmationAttemptID.String()), instance.BusinessKey)
}

func (h *confirmSagaHarness) loadConfirmationSagaByAttempt(attemptID model.ConfirmationAttemptID) (model.SagaInstance, error) {
	h.t.Helper()

	var instance model.SagaInstance
	err := h.pool.Conn(h.ctx, func(ctx context.Context, conn repo.Conn) error {
		sagaQ := h.repos.Saga.Conn(conn)
		row, getErr := sagaQ.GetByTypeAndBusinessKey(
			ctx,
			model.SagaType("checkout_confirmation"),
			model.SagaBusinessKey(attemptID.String()),
		)
		if getErr != nil {
			return getErr
		}

		instance = row
		return nil
	})

	return instance, err
}

func (h *confirmSagaHarness) oldestConfirmationSagaForSession(sessionID model.CheckoutSessionID) model.SagaInstance {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var sagaIDStr string
	err := testDB.QueryRow(ctx, `
		SELECT id
		FROM bnpl.saga_instances
		WHERE saga_type = 'checkout_confirmation'
		  AND subject_type = 'checkout_session'
		  AND subject_id = $1
		ORDER BY created_at ASC
		LIMIT 1
	`, sessionID.String()).Scan(&sagaIDStr)
	require.NoError(h.t, err)

	return h.loadSaga(model.SagaID(uuid.MustParse(sagaIDStr)))
}

func (h *confirmSagaHarness) countConfirmationSagasForSession(sessionID model.CheckoutSessionID) int {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := testDB.QueryRow(ctx, `
		SELECT count(*)
		FROM bnpl.saga_instances
		WHERE saga_type = 'checkout_confirmation'
		  AND subject_type = 'checkout_session'
		  AND subject_id = $1
	`, sessionID.String()).Scan(&count)
	require.NoError(h.t, err)

	return count
}

func updateCreditAccountLimit(t *testing.T, customerID model.CustomerID, limitMinor uint64) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := testDB.Exec(ctx, `
		UPDATE bnpl.credit_accounts
		SET limit_amount_minor = $2, updated_at = now()
		WHERE customer_id = $1
	`, customerID, limitMinor)
	require.NoError(t, err)
}

// bumpCheckoutEvaluationGeneration mirrors production BumpEvaluationGenerationCAS:
// advances evaluation_generation and clears decision_id so Confirm can be
// exercised with a still-valid positive stale generation.
func bumpCheckoutEvaluationGeneration(t *testing.T, sessionID model.CheckoutSessionID) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tag, err := testDB.Exec(ctx, `
		UPDATE bnpl.checkout_sessions
		SET evaluation_generation = evaluation_generation + 1,
			decision_id = NULL,
			otp_issue_id = NULL,
			otp_verifier = NULL,
			otp_expires_at = NULL,
			otp_verified_at = NULL,
			otp_attempt_count = 0,
			updated_at = now()
		WHERE id = $1
		  AND status = 'open'
		  AND contract_id IS NULL
	`, sessionID)
	require.NoError(t, err)
	require.EqualValues(t, 1, tag.RowsAffected())
}

func (h *confirmSagaHarness) queryCheckoutPaymentAttempt(contractID model.ContractID) model.PaymentAttempt {
	h.t.Helper()

	var attempt model.PaymentAttempt
	err := h.pool.Conn(h.ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			paymentAttemptQ := h.repos.PaymentAttempt.Tx(tx)
			row, getErr := paymentAttemptQ.GetByContractIDAndPurposeWithItems(
				ctx,
				contractID,
				model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
			)
			if getErr != nil {
				return getErr
			}

			attempt = row
			return nil
		})
	})
	require.NoError(h.t, err)

	return attempt
}

func (h *confirmSagaHarness) countCheckoutPaymentAttempts(contractID model.ContractID) int {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := testDB.QueryRow(ctx, `
		SELECT count(*)
		FROM bnpl.payment_attempts
		WHERE contract_id = $1
		  AND purpose = $2
	`, contractID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose).Scan(&count)
	require.NoError(h.t, err)

	return count
}

func (h *confirmSagaHarness) buildCheckoutPaymentProcessInput(
	attempt model.PaymentAttempt,
	contract model.Contract,
	eventID string,
) financialcorepaymentresultuc.ProcessInput {
	h.t.Helper()

	require.NotNil(h.t, attempt.FinancialPaymentIntentID)
	intentID := *attempt.FinancialPaymentIntentID

	// Checkout payment identifiers are durably snapshotted on the Contract
	// after CreateCheckoutPayment; the attempt may only hold the intent id.
	require.NotNil(h.t, contract.FinancialPaymentSessionID)
	require.NotNil(h.t, contract.FinancialInvoiceID)
	sessionID := *contract.FinancialPaymentSessionID
	invoiceID := *contract.FinancialInvoiceID

	fcAttemptID := ""
	if attempt.FinancialPaymentAttemptID != nil {
		fcAttemptID = *attempt.FinancialPaymentAttemptID
	} else if contract.FinancialPaymentAttemptID != nil {
		fcAttemptID = *contract.FinancialPaymentAttemptID
	}

	require.NotEmpty(h.t, fcAttemptID)

	h.fc.setPaymentAuthoritativeSucceeded(
		intentID,
		attempt.AmountMinor,
		string(attempt.Currency),
		h.clock.Now().UTC(),
	)

	return financialcorepaymentresultuc.ProcessInput{
		EventID:          eventID,
		EventType:        "financial_core.payment_result.v1",
		EventVersion:     "1",
		SourceService:    "financial-core",
		RawPayload:       json.RawMessage(`{"event_id":"` + eventID + `"}`),
		PaymentPurpose:   model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		ContractID:       contract.ID,
		InstallmentID:    attempt.InstallmentID,
		InvoiceID:        invoiceID,
		PaymentIntentID:  intentID,
		PaymentAttemptID: fcAttemptID,
		PaymentSessionID: sessionID,
		PaymentStatus:    model.SucceededFinancialPaymentResultStatus,
		PaidAmountMinor:  attempt.AmountMinor,
		Currency:         attempt.Currency,
		PaidAt:           h.clock.Now().UTC(),
		EventOccurredAt:  h.clock.Now().UTC(),
		SourceType:       "bnpl_contract",
		SourceID:         contract.ID.String(),
	}
}

func (h *confirmSagaHarness) buildCheckoutPaymentFailureProcessInput(
	attempt model.PaymentAttempt,
	contract model.Contract,
	eventID string,
) financialcorepaymentresultuc.ProcessInput {
	h.t.Helper()

	require.NotNil(h.t, attempt.FinancialPaymentIntentID)
	intentID := *attempt.FinancialPaymentIntentID

	// buildCheckoutPaymentProcessInput records authoritative success. Apply the
	// failure afterwards so GetPaymentStatus does not overwrite this event.
	input := h.buildCheckoutPaymentProcessInput(attempt, contract, eventID)
	h.fc.setPaymentAuthoritativeFailed(intentID)
	input.PaymentStatus = model.FailedFinancialPaymentResultStatus
	failureCode := "card_declined"
	input.FailureCode = &failureCode

	return input
}

func (h *confirmSagaHarness) processPaymentResult(input financialcorepaymentresultuc.ProcessInput) financialcorepaymentresultuc.ProcessOutput {
	h.t.Helper()

	output, err := h.PaymentResult.Process(h.ctx, input)
	require.NoError(h.t, err)

	return output
}

func (h *confirmSagaHarness) driveSagaExpectAttemptFence(oldSagaID model.SagaID, sessionBefore model.CheckoutSession) {
	h.t.Helper()

	err := h.SagaExecutor.ExecuteByID(h.ctx, oldSagaID)
	if err != nil && !errors.Is(err, repo.ErrSagaNotRunnable) && !errors.Is(err, repo.ErrSagaVersionConflict) {
		require.NoError(h.t, err)
	}

	after := h.querySession(sessionBefore.ID)
	require.Equal(h.t, sessionBefore.Status, after.Status)
	if sessionBefore.ConfirmationAttemptID != nil {
		require.Equal(h.t, *sessionBefore.ConfirmationAttemptID, *after.ConfirmationAttemptID)
	}

	if sessionBefore.ContractID != nil && after.ContractID != nil {
		require.Equal(h.t, *sessionBefore.ContractID, *after.ContractID)
	}
}

type checkoutPaymentAttemptSnapshot struct {
	ID             model.PaymentAttemptID
	State          model.PaymentAttemptState
	IdempotencyKey string
	AmountMinor    uint64
	Currency       model.Currency
}

func (h *confirmSagaHarness) listCheckoutPaymentAttempts(contractID model.ContractID) []checkoutPaymentAttemptSnapshot {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := testDB.Query(ctx, `
		SELECT id, state, idempotency_key, amount_minor, currency
		FROM bnpl.payment_attempts
		WHERE contract_id = $1
		  AND purpose = $2
		ORDER BY created_at ASC, id ASC
	`, contractID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose)
	require.NoError(h.t, err)
	defer rows.Close()

	var attempts []checkoutPaymentAttemptSnapshot
	for rows.Next() {
		var (
			id     string
			row    checkoutPaymentAttemptSnapshot
			amount int64
		)
		require.NoError(h.t, rows.Scan(&id, &row.State, &row.IdempotencyKey, &amount, &row.Currency))
		parsed, parseErr := uuid.Parse(id)
		require.NoError(h.t, parseErr)
		row.ID = model.PaymentAttemptID(parsed)
		require.GreaterOrEqual(h.t, amount, int64(0))
		row.AmountMinor = uint64(amount)
		attempts = append(attempts, row)
	}
	require.NoError(h.t, rows.Err())

	return attempts
}

func (h *confirmSagaHarness) countExposureIncreases(contractID model.ContractID) (count int, amountMinor int64) {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := testDB.QueryRow(ctx, `
		SELECT count(*), COALESCE(sum(amount_minor), 0)
		FROM bnpl.credit_transactions
		WHERE type = 'exposure_increased'
		  AND reason_code = 'contract_activated'
		  AND reference_type = 'contract'
		  AND reference_id = $1
	`, contractID).Scan(&count, &amountMinor)
	require.NoError(h.t, err)

	return count, amountMinor
}

func (h *confirmSagaHarness) creditExposureMinor(customerID model.CustomerID) int64 {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var exposure int64
	err := testDB.QueryRow(ctx, `
		SELECT exposure_amount_minor
		FROM bnpl.credit_accounts
		WHERE customer_id = $1
	`, customerID).Scan(&exposure)
	require.NoError(h.t, err)

	return exposure
}

func (h *confirmSagaHarness) countPaidInstallments(contractID model.ContractID) (count int, paidAmount int64) {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := testDB.QueryRow(ctx, `
		SELECT count(*), COALESCE(sum(paid_amount_minor), 0)
		FROM bnpl.installments
		WHERE contract_id = $1
		  AND status = 'paid'
	`, contractID).Scan(&count, &paidAmount)
	require.NoError(h.t, err)

	return count, paidAmount
}

func (h *confirmSagaHarness) countOutboxEvents(eventType string, messageKey string) int {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := testDB.QueryRow(ctx, `
		SELECT count(*)
		FROM bnpl.integration_outbox
		WHERE event_type = $1
		  AND message_key = $2
	`, eventType, messageKey).Scan(&count)
	require.NoError(h.t, err)

	return count
}

func (h *confirmSagaHarness) inboxStatus(sourceService string, eventID string) (status string, count int) {
	h.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := testDB.QueryRow(ctx, `
		SELECT COALESCE(max(status), ''), count(*)
		FROM bnpl.integration_inbox
		WHERE source_service = $1
		  AND event_id = $2
	`, sourceService, eventID).Scan(&status, &count)
	require.NoError(h.t, err)

	return status, count
}
