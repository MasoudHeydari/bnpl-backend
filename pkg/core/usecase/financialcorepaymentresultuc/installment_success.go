package financialcorepaymentresultuc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/paymentapplication"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func (uc *UseCase) applySucceededInstallmentPaymentResult(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	contractQ repo.ContractTxQueryer,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	accountQ repo.CreditAccountTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
	attempt model.PaymentAttempt,
	now time.Time,
	output *ProcessOutput,
) error {
	ack := installmentAckContext(input, purpose, &attempt)

	if attempt.State == model.ReconciliationRequiredPaymentAttemptState {
		return uc.persistInstallmentPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			ack,
			reconciliationReasonValue(attempt.ReconciliationReason),
			now,
			output,
		)
	}

	if attempt.State != model.CreatedPaymentAttemptState &&
		attempt.State != model.SubmittedPaymentAttemptState &&
		attempt.State != model.SucceededPaymentAttemptState {
		return uc.persistInstallmentPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			ack,
			model.InvalidPaymentAttemptStatePaymentReconciliationReason,
			now,
			output,
		)
	}

	if err := validateFinancialPaymentIntentMatchesAttempt(attempt, input); err != nil {
		return uc.persistInstallmentPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			now,
			output,
		)
	}

	// Lock the contract and installment before deciding whether the payment is a new
	// application or an equivalent already-applied payment.
	contractRow, err := contractQ.LockByID(ctx, input.ContractID)
	if err != nil {
		if isContractNotFoundError(err) {
			return uc.persistInstallmentPaymentReconciliation(
				ctx,
				inboxQ,
				outboxQ,
				paymentAttemptQ,
				inboxID,
				input,
				ack,
				model.ContractNotFoundPaymentReconciliationReason,
				now,
				output,
			)
		}

		return err
	}

	installmentRow, err := installmentQ.LockByID(ctx, input.InstallmentID)
	if err != nil {
		if isInstallmentNotFoundError(err) {
			return uc.persistInstallmentPaymentReconciliation(
				ctx,
				inboxQ,
				outboxQ,
				paymentAttemptQ,
				inboxID,
				input,
				ack,
				model.InstallmentNotFoundPaymentReconciliationReason,
				now,
				output,
			)
		}

		return err
	}

	payableAmount, snapshotOK := model.PayableAmountFromItems(attempt.Items)
	if !snapshotOK || payableAmount == 0 || attempt.AmountMinor != payableAmount {
		return uc.persistInstallmentPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			ack,
			model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
			now,
			output,
		)
	}

	if !installmentBelongsToContract(installmentRow, input.ContractID) {
		return uc.persistInstallmentPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			now,
			output,
		)
	}

	identityMatches := duplicateSuccessIdentityMatches(attempt, input)

	// A payment attempt already marked succeeded must be proven applied by the local
	// obligation before acknowledging already_applied. Current exposure and remaining
	// principal are intentionally not required here because an applied payment already
	// reduced them.
	if attempt.State == model.SucceededPaymentAttemptState {
		if !identityMatches {
			return uc.persistInstallmentPaymentReconciliation(
				ctx,
				inboxQ,
				outboxQ,
				paymentAttemptQ,
				inboxID,
				input,
				ack,
				model.DuplicateSuccessfulPaymentPaymentReconciliationReason,
				now,
				output,
			)
		}

		if installmentRow.Status != model.PaidInstallmentStatus {
			return uc.persistInstallmentPaymentReconciliation(
				ctx,
				inboxQ,
				outboxQ,
				paymentAttemptQ,
				inboxID,
				input,
				ack,
				model.InvalidInstallmentStatePaymentReconciliationReason,
				now,
				output,
			)
		}

		if !isContractStateCompatibleWithAppliedPayment(contractRow.Status) {
			return uc.persistInstallmentPaymentReconciliation(
				ctx,
				inboxQ,
				outboxQ,
				paymentAttemptQ,
				inboxID,
				input,
				ack,
				model.InvalidContractStatePaymentReconciliationReason,
				now,
				output,
			)
		}

		return uc.finishAlreadyAppliedInstallmentPayment(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			ack,
			attempt,
			contractRow,
			installmentRow,
			now,
			output,
		)
	}

	// Recovery: a submitted attempt whose installment is already paid by the same payment
	// is acknowledged already_applied using the historical application timestamp. The
	// contract must be in a state compatible with an applied payment; otherwise the local
	// obligation state does not prove the payment was applied.
	if installmentRow.Status == model.PaidInstallmentStatus {
		if !identityMatches {
			return uc.persistInstallmentPaymentReconciliation(
				ctx,
				inboxQ,
				outboxQ,
				paymentAttemptQ,
				inboxID,
				input,
				ack,
				model.InstallmentAlreadyPaidByOtherPaymentPaymentReconciliationReason,
				now,
				output,
			)
		}

		if !isContractStateCompatibleWithAppliedPayment(contractRow.Status) {
			return uc.persistInstallmentPaymentReconciliation(
				ctx,
				inboxQ,
				outboxQ,
				paymentAttemptQ,
				inboxID,
				input,
				ack,
				model.InvalidContractStatePaymentReconciliationReason,
				now,
				output,
			)
		}

		return uc.finishAlreadyAppliedInstallmentPayment(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			ack,
			attempt,
			contractRow,
			installmentRow,
			now,
			output,
		)
	}

	validation := validateInstallmentSuccessPreconditions(
		input,
		attempt,
		contractRow,
		installmentRow,
		payableAmount,
	)
	if validation.reason != nil {
		return uc.persistInstallmentPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			ack,
			*validation.reason,
			now,
			output,
		)
	}

	lockedCharges, err := installmentChargeQ.LockByIDs(ctx, chargeSourceIDsFromSnapshot(attempt.Items))
	if err != nil {
		return err
	}

	creditAccount, accountErr := lockCreditAccountForPrincipalRelease(
		ctx, accountQ, contractRow, installmentRow.PrincipalAmountMinor,
	)
	if accountErr != nil {
		var appErr *ReconciliationApplicationError
		if errors.As(accountErr, &appErr) {
			return uc.persistInstallmentPaymentReconciliation(
				ctx,
				inboxQ,
				outboxQ,
				paymentAttemptQ,
				inboxID,
				input,
				ack,
				appErr.Reason,
				now,
				output,
			)
		}

		return accountErr
	}

	if reason, ok := validatePaymentAttemptSnapshot(attempt, installmentRow, lockedCharges, payableAmount); !ok {
		return uc.persistInstallmentPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			ack,
			reason,
			now,
			output,
		)
	}

	if contractRow.RemainingPrincipalAmountMinor < installmentRow.PrincipalAmountMinor {
		return uc.persistInstallmentPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			now,
			output,
		)
	}

	return uc.applyNewInstallmentPayment(
		ctx,
		inboxQ,
		outboxQ,
		paymentAttemptQ,
		contractQ,
		installmentQ,
		installmentChargeQ,
		accountQ,
		creditAccount,
		inboxID,
		input,
		ack,
		attempt,
		contractRow,
		installmentRow,
		payableAmount,
		now,
		output,
	)
}

func (uc *UseCase) finishAlreadyAppliedInstallmentPayment(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	ack applicationAckContext,
	attempt model.PaymentAttempt,
	contractRow model.Contract,
	installmentRow model.Installment,
	now time.Time,
	output *ProcessOutput,
) error {
	// Resolve the historical application timestamp before any mutation so a recovered
	// submitted attempt uses Installment.PaidAt rather than the acknowledgement time.
	appliedAt := originalApplicationTimestamp(&attempt, &installmentRow, &contractRow)

	if attempt.State == model.CreatedPaymentAttemptState ||
		attempt.State == model.SubmittedPaymentAttemptState {
		markAppliedAt := now
		if appliedAt != nil {
			markAppliedAt = *appliedAt
		}

		succeededAttempt, markErr := paymentAttemptQ.MarkSucceeded(
			ctx,
			attempt.ID,
			optionalProviderAttemptID(input.PaymentAttemptID),
			input.PaidAt.UTC(),
			markAppliedAt,
			now,
		)
		if markErr != nil {
			if cerr.IsCode(markErr, cerr.CodeConflict) {
				return uc.persistInstallmentPaymentReconciliation(
					ctx,
					inboxQ,
					outboxQ,
					paymentAttemptQ,
					inboxID,
					input,
					ack,
					model.InvalidInstallmentStatePaymentReconciliationReason,
					now,
					output,
				)
			}

			return markErr
		}

		attempt = succeededAttempt
	}

	ack.Attempt = &attempt
	installmentStatus := installmentRow.Status
	contractStatus := contractRow.Status
	ack.InstallmentStatus = &installmentStatus
	ack.ContractStatus = &contractStatus

	return uc.finishSuccessfulPaymentApplication(
		ctx,
		inboxQ,
		outboxQ,
		inboxID,
		input,
		ack,
		paymentapplication.AlreadyAppliedApplicationResult,
		appliedAt,
		now,
		output,
	)
}

// isContractStateCompatibleWithAppliedPayment reports whether a contract state can back an
// already-applied installment payment acknowledgement.
func isContractStateCompatibleWithAppliedPayment(status model.ContractStatus) bool {
	switch status {
	case model.ActiveContractStatus,
		model.CancelingContractStatus,
		model.CanceledContractStatus,
		model.CompletedContractStatus:
		return true

	default:
		return false
	}
}

func (uc *UseCase) applyNewInstallmentPayment(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	contractQ repo.ContractTxQueryer,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	accountQ repo.CreditAccountTxQueryer,
	creditAccount model.CreditAccount,
	inboxID model.InboxID,
	input ProcessInput,
	ack applicationAckContext,
	attempt model.PaymentAttempt,
	contractRow model.Contract,
	installmentRow model.Installment,
	payableAmount uint64,
	now time.Time,
	output *ProcessOutput,
) error {
	const op = "financialcorepaymentresultuc.UseCase.applyNewInstallmentPayment"

	succeededAttempt, err := paymentAttemptQ.MarkSucceeded(
		ctx,
		attempt.ID,
		optionalProviderAttemptID(input.PaymentAttemptID),
		input.PaidAt.UTC(),
		now,
		now,
	)
	if err != nil {
		return err
	}

	paidInstallment, err := installmentQ.MarkPaid(
		ctx,
		installmentRow.ID,
		contractRow.ID,
		payableAmount,
		input.PaidAt.UTC(),
		now,
	)
	if err != nil {
		return err
	}

	debitChargeIDs := debitChargeSourceIDsFromSnapshot(attempt.Items)
	if len(debitChargeIDs) > 0 {
		affected, markChargeErr := installmentChargeQ.MarkSnapshottedChargeIDsPaid(
			ctx,
			debitChargeIDs,
			input.PaidAt.UTC(),
			now,
		)
		if markChargeErr != nil {
			return markChargeErr
		}

		if affected != int64(len(debitChargeIDs)) {
			return cerr.InternalError(fmt.Errorf("%s: snapshotted charge settlement affected row count mismatch", op))
		}
	}

	if err = reduceCreditExposureForInstallment(
		ctx,
		accountQ,
		creditAccount,
		installmentRow,
		contractRow.Currency,
		input.PaidAt,
		now,
	); err != nil {
		return err
	}

	updatedContract := contractRow
	if contractRow.RemainingPrincipalAmountMinor > 0 {
		updatedContract, err = contractQ.ReduceRemainingPrincipal(
			ctx,
			contractRow.ID,
			installmentRow.PrincipalAmountMinor,
			now,
		)
		if err != nil {
			return err
		}
	}

	canComplete, completeErr := canCompleteContract(
		updatedContract,
		paidInstallment,
		installmentQ,
		installmentChargeQ,
		paymentAttemptQ,
		ctx,
	)
	if completeErr != nil {
		return completeErr
	}

	finalContractStatus := updatedContract.Status
	if canComplete {
		if err = updatedContract.Status.ValidateTransitionTo(model.CompletedContractStatus); err != nil {
			return cerr.InternalError(err)
		}

		completedContract, completeMarkErr := contractQ.MarkCompleted(ctx, contractRow.ID, now, now)
		if completeMarkErr != nil {
			return completeMarkErr
		}

		finalContractStatus = completedContract.Status
	}

	ack.Attempt = &succeededAttempt
	installmentStatus := paidInstallment.Status
	ack.InstallmentStatus = &installmentStatus
	ack.ContractStatus = &finalContractStatus
	appliedAtValue := now.UTC()

	if err := uc.insertInstallmentRepaymentAudit(
		ctx,
		outboxQ,
		input,
		installmentRepaymentAuditFacts{
			InboxID:                     inboxID,
			PreviousInstallmentStatus:   installmentRow.Status,
			CurrentInstallment:          paidInstallment,
			PreviousPaymentAttemptState: attempt.State,
			CurrentPaymentAttempt:       succeededAttempt,
			PreviousContractStatus:      contractRow.Status,
			CurrentContractStatus:       finalContractStatus,
			RemainingPrincipalMinor:     updatedContract.RemainingPrincipalAmountMinor,
			ContractCompleted:           canComplete,
			AppliedAt:                   appliedAtValue,
			CustomerID:                  contractRow.CustomerID,
			ContractID:                  contractRow.ID,
		},
		now,
	); err != nil {
		return err
	}

	return uc.finishSuccessfulPaymentApplication(
		ctx,
		inboxQ,
		outboxQ,
		inboxID,
		input,
		ack,
		paymentapplication.AppliedApplicationResult,
		&appliedAtValue,
		now,
		output,
	)
}

func canCompleteContract(
	contractRow model.Contract,
	paidInstallment model.Installment,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	ctx context.Context,
) (bool, error) {
	if contractRow.Status != model.ActiveContractStatus {
		return false, nil
	}

	if contractRow.RemainingPrincipalAmountMinor != 0 {
		return false, nil
	}

	installments, err := installmentQ.ListByContractID(ctx, contractRow.ID)
	if err != nil {
		return false, err
	}

	for i := range installments {
		if installments[i].ID == paidInstallment.ID {
			installments[i] = paidInstallment
		}
	}

	if !allRequiredInstallmentsPaid(installments) {
		return false, nil
	}

	unpaidCharges, err := installmentChargeQ.CountUnpaidCustomerDebitChargesByContractID(ctx, contractRow.ID)
	if err != nil {
		return false, err
	}

	if unpaidCharges > 0 {
		return false, nil
	}

	hasBlockingAttempt, err := paymentAttemptQ.HasBlockingAttemptForContract(ctx, contractRow.ID)
	if err != nil {
		return false, err
	}

	return !hasBlockingAttempt, nil
}

type installmentSuccessValidation struct {
	reason *model.PaymentReconciliationReason
}

func validateInstallmentSuccessPreconditions(
	input ProcessInput,
	attempt model.PaymentAttempt,
	contractRow model.Contract,
	installmentRow model.Installment,
	payableAmount uint64,
) installmentSuccessValidation {
	if contractRow.Status != model.ActiveContractStatus {
		reason := model.InvalidContractStatePaymentReconciliationReason
		return installmentSuccessValidation{reason: &reason}
	}

	if !installmentBelongsToContract(installmentRow, input.ContractID) {
		reason := model.PaymentReferenceMismatchPaymentReconciliationReason
		return installmentSuccessValidation{reason: &reason}
	}

	if installmentRow.InstallmentOrder <= 1 {
		reason := model.InvalidInstallmentStatePaymentReconciliationReason
		return installmentSuccessValidation{reason: &reason}
	}

	if installmentRow.Status == model.PaidInstallmentStatus {
		reason := model.InstallmentAlreadyPaidByOtherPaymentPaymentReconciliationReason
		return installmentSuccessValidation{reason: &reason}
	}

	if !isPayableInstallmentStatus(installmentRow.Status) {
		reason := model.InvalidInstallmentStatePaymentReconciliationReason
		return installmentSuccessValidation{reason: &reason}
	}

	if input.PaidAmountMinor != payableAmount {
		reason := model.PaymentAmountMismatchPaymentReconciliationReason
		return installmentSuccessValidation{reason: &reason}
	}

	if input.Currency != contractRow.Currency || input.Currency != installmentRow.Currency {
		reason := model.PaymentCurrencyMismatchPaymentReconciliationReason
		return installmentSuccessValidation{reason: &reason}
	}

	if err := installmentRow.Status.ValidateTransitionTo(model.PaidInstallmentStatus); err != nil {
		reason := model.InvalidInstallmentStatePaymentReconciliationReason
		return installmentSuccessValidation{reason: &reason}
	}

	return installmentSuccessValidation{}
}
