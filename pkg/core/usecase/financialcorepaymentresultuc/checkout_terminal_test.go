package financialcorepaymentresultuc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func TestFinishCheckoutTerminalPaymentResult_ValidatesStoredPaymentIdentity(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	occurredAt := now.Add(-time.Minute)
	contractID := model.ContractID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	inboxID := model.InboxID(uuid.New())
	intentID := "intent-1"
	sessionID := "session-1"

	baseAttempt := model.PaymentAttempt{
		ID:                        attemptID,
		ContractID:                contractID,
		Purpose:                   model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		State:                     model.SubmittedPaymentAttemptState,
		FinancialPaymentIntentID:  &intentID,
		FinancialPaymentSessionID: &sessionID,
	}

	t.Run("matching terminal result is processed", func(t *testing.T) {
		t.Parallel()

		tx := mock.NewTx(t)
		paymentAttemptRepo := mock.NewPaymentAttempt(t)
		paymentAttemptQ := mock.NewPaymentAttemptTxQueryer(t)
		contractRepo := mock.NewContract(t)
		contractQ := mock.NewContractTxQueryer(t)
		installmentRepo := mock.NewInstallment(t)
		installmentQ := mock.NewInstallmentTxQueryer(t)
		installmentChargeRepo := mock.NewInstallmentCharge(t)
		installmentChargeQ := mock.NewInstallmentChargeTxQueryer(t)
		inboxQ := mock.NewInboxTxQueryer(t)
		outboxQ := mock.NewOutboxTxQueryer(t)

		paymentAttemptRepo.EXPECT().Tx(tx).Return(paymentAttemptQ).Once()
		paymentAttemptQ.EXPECT().
			GetByFinancialPaymentIntentIDWithItems(ctx, intentID).
			Return(baseAttempt, nil).Once()
		contractRepo.EXPECT().Tx(tx).Return(contractQ).Once()
		contractQ.EXPECT().LockByID(ctx, contractID).Return(model.Contract{
			ID:                        contractID,
			Status:                    model.PaymentPendingContractStatus,
			FinancialPaymentIntentID:  &intentID,
			FinancialPaymentSessionID: &sessionID,
		}, nil).Once()
		paymentAttemptQ.EXPECT().
			LockByIDWithItems(ctx, attemptID).
			Return(baseAttempt, nil).Once()
		paymentAttemptQ.EXPECT().
			EnrichFinancialPaymentIdentityIfAbsent(ctx, attemptID, intentID, &sessionID, (*string)(nil), now).
			Return(baseAttempt, nil).Once()
		paymentAttemptQ.EXPECT().
			MarkFailed(ctx, attemptID, "unknown", occurredAt, now).
			Return(baseAttempt, nil).Once()
		contractQ.EXPECT().MarkCanceledFromPaymentPending(ctx, contractID, occurredAt, now).
			Return(model.Contract{}, nil).Once()
		installmentRepo.EXPECT().Tx(tx).Return(installmentQ).Once()
		installmentQ.EXPECT().ExpireOfferedByContractID(ctx, contractID, now).Return(int64(0), nil).Once()
		installmentChargeRepo.EXPECT().Tx(tx).Return(installmentChargeQ).Once()
		installmentChargeQ.EXPECT().CancelOfferedByContractID(ctx, contractID, now).Return(int64(0), nil).Once()
		outboxQ.EXPECT().Create(ctx, testifymock.MatchedBy(func(message repo.CreateOutboxMessage) bool {
			if message.Destination != testAuditDestination || message.EventType != audit.EventType {
				return false
			}

			var event audit.Event
			if err := json.Unmarshal(message.Payload, &event); err != nil {
				return false
			}

			return event.Data.Action == auditActionPaymentAttemptMarkFailed &&
				event.Data.Resource.ID == attemptID.String() &&
				audit.ValidateEvent(event) == nil
		})).Return(model.OutboxMessage{}, nil).Once()
		inboxQ.EXPECT().MarkProcessed(ctx, inboxID, now, now).
			Return(model.Inbox{ID: inboxID, Status: model.ProcessedInboxStatus}, nil).Once()

		uc := &UseCase{
			paymentAttempt:    paymentAttemptRepo,
			contract:          contractRepo,
			installment:       installmentRepo,
			installmentCharge: installmentChargeRepo,
			checkoutSessionUC: noopCheckoutSessionPaymentHook{},
			idGenerator:       testIDGenerator{value: "audit-event-1"},
			auditDestination:  testAuditDestination,
		}
		var output ProcessOutput
		err := uc.finishCheckoutTerminalPaymentResult(
			ctx,
			tx,
			inboxQ,
			outboxQ,
			inboxID,
			ProcessInput{
				EventID:          "fc-evt-1",
				EventType:        "payment.result",
				EventVersion:     "1",
				SourceService:    "financial-core",
				ContractID:       contractID,
				PaymentIntentID:  intentID,
				PaymentSessionID: sessionID,
				PaymentStatus:    model.FailedFinancialPaymentResultStatus,
				EventOccurredAt:  occurredAt,
			},
			now,
			ProcessResultFailureRecorded,
			&output,
		)
		require.NoError(t, err)
		require.Equal(t, ProcessResultFailureRecorded, output.Result)
	})

	t.Run("mismatched intent is retained as failed inbox evidence", func(t *testing.T) {
		t.Parallel()

		tx := mock.NewTx(t)
		paymentAttemptRepo := mock.NewPaymentAttempt(t)
		paymentAttemptQ := mock.NewPaymentAttemptTxQueryer(t)
		contractRepo := mock.NewContract(t)
		contractQ := mock.NewContractTxQueryer(t)
		inboxQ := mock.NewInboxTxQueryer(t)
		outboxQ := mock.NewOutboxTxQueryer(t)

		paymentAttemptRepo.EXPECT().Tx(tx).Return(paymentAttemptQ).Once()
		// Intent lookup key differs from the attempt's stored intent; validation rejects it.
		paymentAttemptQ.EXPECT().
			GetByFinancialPaymentIntentIDWithItems(ctx, "other-intent").
			Return(baseAttempt, nil).Once()
		contractRepo.EXPECT().Tx(tx).Return(contractQ).Once()
		contractQ.EXPECT().LockByID(ctx, contractID).Return(model.Contract{
			ID:                       contractID,
			Status:                   model.PaymentPendingContractStatus,
			FinancialPaymentIntentID: &intentID,
		}, nil).Once()
		paymentAttemptQ.EXPECT().
			LockByIDWithItems(ctx, attemptID).
			Return(baseAttempt, nil).Once()
		inboxQ.EXPECT().MarkFailed(
			ctx,
			inboxID,
			"payment_reference_mismatch",
			"terminal checkout payment result does not match the local payment attempt",
			now,
			now,
		).Return(model.Inbox{ID: inboxID, Status: model.FailedInboxStatus}, nil).Once()
		outboxQ.EXPECT().Create(ctx, testifymock.MatchedBy(func(message repo.CreateOutboxMessage) bool {
			var event audit.Event
			if err := json.Unmarshal(message.Payload, &event); err != nil {
				return false
			}

			return event.Data.Action == auditActionPaymentResultRecordUnmatchedTerminalResult &&
				audit.ValidateEvent(event) == nil
		})).Return(model.OutboxMessage{}, nil).Once()

		uc := &UseCase{
			paymentAttempt:   paymentAttemptRepo,
			contract:         contractRepo,
			idGenerator:      testIDGenerator{value: "audit-event-2"},
			auditDestination: testAuditDestination,
		}
		var output ProcessOutput
		err := uc.finishCheckoutTerminalPaymentResult(
			ctx,
			tx,
			inboxQ,
			outboxQ,
			inboxID,
			ProcessInput{
				EventID:         "fc-evt-2",
				EventType:       "payment.result",
				EventVersion:    "1",
				SourceService:   "financial-core",
				ContractID:      contractID,
				PaymentIntentID: "other-intent",
				PaymentStatus:   model.FailedFinancialPaymentResultStatus,
				EventOccurredAt: occurredAt,
			},
			now,
			ProcessResultFailureRecorded,
			&output,
		)
		require.NoError(t, err)
		require.Equal(t, ProcessResultTerminalResultUnmatched, output.Result)
	})
}
