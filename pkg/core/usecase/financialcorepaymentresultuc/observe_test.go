package financialcorepaymentresultuc

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func newObserveCommittedUseCase(t *testing.T) (*UseCase, *mock.FinancialCorePaymentResultObserver) {
	t.Helper()

	observer := mock.NewFinancialCorePaymentResultObserver(t)

	return &UseCase{observer: observer}, observer
}

func TestObserveCommitted_CheckoutPaymentApplied_ActivatesContractAfterCommit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	uc, observer := newObserveCommittedUseCase(t)
	contractID := model.ContractID(uuid.New())
	input := ProcessInput{
		EventID:       "event-1",
		ContractID:    contractID,
		PaymentStatus: model.SucceededFinancialPaymentResultStatus,
	}

	observer.EXPECT().ApplicationSucceeded(ctx, "event-1", contractID).Once()
	observer.EXPECT().ContractActivated(ctx, "event-1", contractID).Once()

	uc.observeCommitted(
		ctx,
		input,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		ProcessOutput{Result: ProcessResultPaymentApplied},
	)
}

func TestObserveCommitted_InboxDuplicate_IsNotANewApplication(t *testing.T) {
	t.Parallel()

	for _, result := range []ProcessResult{
		ProcessResultEventAlreadyProcessed,
		ProcessResultReconciliationRequired,
		ProcessResultPermanentlyRejected,
	} {
		t.Run(string(result), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			uc, observer := newObserveCommittedUseCase(t)
			contractID := model.ContractID(uuid.New())
			input := ProcessInput{EventID: "event-1", ContractID: contractID}

			// Only the duplicate checkpoint is allowed: no application, no
			// activation, no conflict, and no validation-failure checkpoint.
			observer.EXPECT().InboxDuplicateDetected(ctx, "event-1", contractID, string(result)).Once()

			uc.observeCommitted(
				ctx,
				input,
				model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
				ProcessOutput{Result: result, inboxDuplicate: true},
			)
		})
	}
}

func TestObserveCommitted_AlreadyApplied_DoesNotActivateContract(t *testing.T) {
	t.Parallel()

	for _, result := range []ProcessResult{ProcessResultAlreadyApplied, ProcessResultEventAlreadyProcessed} {
		t.Run(string(result), func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			uc, observer := newObserveCommittedUseCase(t)
			contractID := model.ContractID(uuid.New())

			observer.EXPECT().ApplicationAlreadyApplied(ctx, "event-1", contractID).Once()

			uc.observeCommitted(
				ctx,
				ProcessInput{EventID: "event-1", ContractID: contractID},
				model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
				ProcessOutput{Result: result},
			)
		})
	}
}

func TestObserveCommitted_ReconciliationRequired_DoesNotActivateContract(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	uc, observer := newObserveCommittedUseCase(t)
	contractID := model.ContractID(uuid.New())

	observer.EXPECT().ApplicationConflict(ctx, "event-1", contractID, "payment_amount_mismatch").Once()

	uc.observeCommitted(
		ctx,
		ProcessInput{EventID: "event-1", ContractID: contractID},
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		ProcessOutput{Result: ProcessResultReconciliationRequired, outcomeReasonCode: "payment_amount_mismatch"},
	)
}

func TestObserveCommitted_InstallmentRepaymentApplied_DoesNotActivateContract(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	uc, observer := newObserveCommittedUseCase(t)
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())

	observer.EXPECT().ApplicationSucceeded(ctx, "event-1", contractID).Once()
	observer.EXPECT().InstallmentRepaymentApplied(ctx, "event-1", contractID, installmentID).Once()

	uc.observeCommitted(
		ctx,
		ProcessInput{EventID: "event-1", ContractID: contractID, InstallmentID: installmentID},
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ProcessOutput{Result: ProcessResultPaymentApplied},
	)
}

func TestObserveCommitted_InstallmentTerminalFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	uc, observer := newObserveCommittedUseCase(t)
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())

	observer.EXPECT().TerminalFailure(ctx, "event-1", contractID, model.FailedFinancialPaymentResultStatus).Once()
	observer.EXPECT().InstallmentRepaymentFailed(
		ctx,
		"event-1",
		contractID,
		installmentID,
		model.FailedFinancialPaymentResultStatus,
	).Once()

	uc.observeCommitted(
		ctx,
		ProcessInput{
			EventID:       "event-1",
			ContractID:    contractID,
			InstallmentID: installmentID,
			PaymentStatus: model.FailedFinancialPaymentResultStatus,
		},
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ProcessOutput{Result: ProcessResultFailureRecorded},
	)
}
