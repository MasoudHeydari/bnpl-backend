package financialcorepaymentresultuc

import (
	"context"
	"errors"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

const (
	// unknownValidationReasonCode labels validation failures without a stable
	// application error code.
	unknownValidationReasonCode = "invalid_payment_result"

	// unknownOutcomeReasonCode labels non-success outcomes without a recorded
	// reconciliation or rejection reason.
	unknownOutcomeReasonCode = "unknown"
)

// observedContractID returns the contract targeted by the payment result for
// observability only. A zero ID is returned when the result does not identify
// one.
func observedContractID(input ProcessInput) model.ContractID {
	if !input.ContractID.IsZero() {
		return input.ContractID
	}

	resolved, err := resolveContractID(contractuc.ActivateFromFinancialCorePaymentResultInput{
		ContractID: input.ContractID,
		SourceType: input.SourceType,
		SourceID:   input.SourceID,
	})
	if err != nil {
		return model.ContractID{}
	}

	return resolved
}

func validationReasonCode(err error) string {
	var applicationErr *cerr.Error
	if errors.As(err, &applicationErr) && applicationErr.Code != "" {
		return string(applicationErr.Code)
	}

	return unknownValidationReasonCode
}

// observeCommitted records the outcome of one Process attempt. It must be
// called only after the database transaction committed so success checkpoints
// never describe uncommitted work.
//
// Inbox duplicates are reported as duplicates and never as new applications.
func (uc *UseCase) observeCommitted(
	ctx context.Context,
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
	output ProcessOutput,
) {
	contractID := observedContractID(input)

	if output.inboxDuplicate {
		uc.observer.InboxDuplicateDetected(ctx, input.EventID, contractID, string(output.Result))
		return
	}

	reasonCode := output.outcomeReasonCode
	if reasonCode == "" {
		reasonCode = unknownOutcomeReasonCode
	}

	switch output.Result {
	case ProcessResultPaymentApplied:
		uc.observer.ApplicationSucceeded(ctx, input.EventID, contractID)

		switch purpose {
		case model.CheckoutFirstInstallmentFinancialCorePaymentPurpose:
			uc.observer.ContractActivated(ctx, input.EventID, contractID)

		case model.InstallmentRepaymentFinancialCorePaymentPurpose:
			uc.observer.InstallmentRepaymentApplied(ctx, input.EventID, contractID, input.InstallmentID)
		}

	case ProcessResultAlreadyApplied, ProcessResultEventAlreadyProcessed:
		uc.observer.ApplicationAlreadyApplied(ctx, input.EventID, contractID)

	case ProcessResultFailureRecorded, ProcessResultCancellationRecorded:
		uc.observer.TerminalFailure(ctx, input.EventID, contractID, input.PaymentStatus)

		if purpose == model.InstallmentRepaymentFinancialCorePaymentPurpose {
			uc.observer.InstallmentRepaymentFailed(
				ctx,
				input.EventID,
				contractID,
				input.InstallmentID,
				input.PaymentStatus,
			)
		}

	case ProcessResultReconciliationRequired,
		ProcessResultPayloadConflictRecorded,
		ProcessResultTerminalResultUnmatched:
		uc.observer.ApplicationConflict(ctx, input.EventID, contractID, reasonCode)

	case ProcessResultPermanentlyRejected:
		uc.observer.ResultValidationFailed(ctx, input.EventID, contractID, reasonCode)
	}
}
