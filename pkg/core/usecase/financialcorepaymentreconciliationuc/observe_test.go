package financialcorepaymentreconciliationuc

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestObserveCommitted_InboxDuplicate_IsNotANewResolution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	observer := mock.NewFinancialCorePaymentReconciliationObserver(t)
	uc := &UseCase{observer: observer}
	attemptID := model.PaymentAttemptID(uuid.New())
	input := ProcessInput{
		EventID: "event-1",
		Data:    installmentpaymentreconciliation.ResultEventData{BNPLPaymentAttemptID: attemptID},
	}

	observer.EXPECT().InboxDuplicateDetected(ctx, "event-1", attemptID, string(ProcessResultAlreadyProcessed)).Once()

	uc.observeCommitted(ctx, input, ProcessOutput{Result: ProcessResultAlreadyProcessed, inboxDuplicate: true})
}

func TestObserveCommitted_ResolvedSucceeded_ObservesResolution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	observer := mock.NewFinancialCorePaymentReconciliationObserver(t)
	uc := &UseCase{observer: observer}
	attemptID := model.PaymentAttemptID(uuid.New())
	input := ProcessInput{
		EventID: "event-1",
		Data:    installmentpaymentreconciliation.ResultEventData{BNPLPaymentAttemptID: attemptID},
	}

	observer.EXPECT().ReconciliationResolved(ctx, "event-1", attemptID, reconciliationOutcomeSucceeded).Once()

	uc.observeCommitted(ctx, input, ProcessOutput{Result: ProcessResultResolvedSucceeded})
}

func TestObserveCommitted_StillReconciliationRequired_SchedulesRetry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	observer := mock.NewFinancialCorePaymentReconciliationObserver(t)
	uc := &UseCase{observer: observer}
	attemptID := model.PaymentAttemptID(uuid.New())
	input := ProcessInput{
		EventID: "event-1",
		Data:    installmentpaymentreconciliation.ResultEventData{BNPLPaymentAttemptID: attemptID},
	}

	observer.EXPECT().ReconciliationRetryScheduled(ctx, "event-1", attemptID).Once()

	uc.observeCommitted(ctx, input, ProcessOutput{Result: ProcessResultStillReconciliationRequired})
}
