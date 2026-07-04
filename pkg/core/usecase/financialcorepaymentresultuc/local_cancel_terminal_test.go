package financialcorepaymentresultuc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestApplyTerminalInstallmentPaymentResult_ResolvesLocalCancel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status model.FinancialPaymentResultStatus
	}{
		{"fc canceled", model.CanceledFinancialPaymentResultStatus},
		{"fc expired mapped to failed", model.FailedFinancialPaymentResultStatus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
			attemptID := model.PaymentAttemptID(uuid.New())
			inboxID := model.InboxID(uuid.New())
			reason := model.SessionCreationRetryExhaustedFailureCode
			attempt := model.PaymentAttempt{
				ID: attemptID,
				Purpose: model.InstallmentRepaymentFinancialCorePaymentPurpose,
				State: model.CanceledPaymentAttemptState,
				FailureCode: &reason,
				ContractID: model.ContractID(uuid.New()),
				InstallmentID: model.InstallmentID(uuid.New()),
			}
			resolved := attempt
			resolved.FinancialOutcomeResolved = true

			paymentAttemptQ := mock.NewPaymentAttemptTxQueryer(t)
			inboxQ := mock.NewInboxTxQueryer(t)
			outboxQ := mock.NewOutboxTxQueryer(t)
			idGenerator := mock.NewIdentifierGenerator(t)
			idGenerator.EXPECT().Generate().Return(uuid.NewString()).Once()

			paymentAttemptQ.EXPECT().ResolveAuthoritativeCancellationOnLocalCancel(
				ctx, attempt.ID, (*string)(nil), now, now,
			).Return(resolved, nil).Once()
			outboxQ.EXPECT().Create(ctx, testifymock.Anything).
				Return(model.OutboxMessage{}, nil).Once()
			inboxQ.EXPECT().MarkProcessed(ctx, inboxID, now, now).
				Return(model.Inbox{ID: inboxID}, nil).Once()

			uc := &UseCase{
				idGenerator: idGenerator,
				auditDestination: "bnpl.audit.events.v1",
			}
			input := ProcessInput{
				EventID: "fc-terminal-" + uuid.NewString(),
				SourceService: "financial_core",
				PaymentStatus: tc.status,
				EventOccurredAt: now,
				ContractID: attempt.ContractID,
				InstallmentID: attempt.InstallmentID,
			}
			var output ProcessOutput
			err := uc.applyTerminalInstallmentPaymentResult(
				ctx, inboxQ, outboxQ, paymentAttemptQ, inboxID,
				input, &attempt, now, &output,
			)
			require.NoError(t, err)
			require.Equal(t, model.CanceledPaymentAttemptState, attempt.State)
			require.True(t, attempt.FinancialOutcomeResolved)
			require.Equal(t, ProcessResultCancellationRecorded, output.Result)
		})
	}
}
