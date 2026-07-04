package financialcorepaymentresultuc

import (
	"context"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func (uc *UseCase) applyTerminalInstallmentPaymentResult(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	attempt *model.PaymentAttempt,
	now time.Time,
	output *ProcessOutput,
) error {
	if attempt == nil {
		return uc.persistUnmatchedTerminalInstallmentResult(
			ctx,
			inboxQ,
			outboxQ,
			inboxID,
			input,
			"unknown_payment_attempt",
			"local payment attempt not found for terminal financial core payment result",
			nil,
			now,
			output,
		)
	}

	if input.PaymentStatus == model.CanceledFinancialPaymentResultStatus &&
		attempt.State == model.CanceledPaymentAttemptState {
		return uc.finishTerminalInbox(
			ctx,
			inboxQ,
			inboxID,
			ProcessResultEventAlreadyProcessed,
			now,
			output,
		)
	}

	if input.PaymentStatus == model.FailedFinancialPaymentResultStatus &&
		attempt.State == model.FailedPaymentAttemptState {
		return uc.finishTerminalInbox(
			ctx,
			inboxQ,
			inboxID,
			ProcessResultEventAlreadyProcessed,
			now,
			output,
		)
	}

	if attempt.State != model.CreatedPaymentAttemptState &&
		attempt.State != model.SubmittedPaymentAttemptState &&
		attempt.State != model.ReconciliationRequiredPaymentAttemptState {
		return uc.persistUnmatchedTerminalInstallmentResult(
			ctx,
			inboxQ,
			outboxQ,
			inboxID,
			input,
			"invalid_payment_attempt_state",
			"terminal financial core payment result does not match local payment attempt state",
			attempt,
			now,
			output,
		)
	}

	if err := validateResultTargetsAttempt(
		*attempt,
		input,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
	); err != nil {
		return uc.persistUnmatchedTerminalInstallmentResult(
			ctx,
			inboxQ,
			outboxQ,
			inboxID,
			input,
			"payment_reference_mismatch",
			"terminal financial core payment result does not match local payment attempt references",
			attempt,
			now,
			output,
		)
	}

	previousState := attempt.State
	enrichedAttempt, enrichErr := enrichAttemptIdentityFromResult(
		ctx,
		paymentAttemptQ,
		*attempt,
		input,
		now,
	)
	if enrichErr != nil {
		return uc.persistUnmatchedTerminalInstallmentResult(
			ctx,
			inboxQ,
			outboxQ,
			inboxID,
			input,
			"payment_reference_mismatch",
			"terminal financial core payment result conflicts with local payment identity",
			attempt,
			now,
			output,
		)
	}

	attempt = &enrichedAttempt

	if input.PaymentStatus == model.CanceledFinancialPaymentResultStatus {
		cancelCode := normalizedFailureCode(input.FailureCode)
		if _, err := paymentAttemptQ.MarkCanceled(
			ctx,
			attempt.ID,
			cancelCode,
			input.EventOccurredAt.UTC(),
			now,
		); err != nil {
			if cerr.IsCode(err, cerr.CodeConflict) {
				return uc.persistUnmatchedTerminalInstallmentResult(
					ctx,
					inboxQ,
					outboxQ,
					inboxID,
					input,
					"invalid_payment_attempt_state",
					"terminal cancellation could not be applied to local payment attempt",
					attempt,
					now,
					output,
				)
			}

			return err
		}

		reasonCode := auditReasonPaymentAttemptCanceled
		if cancelCode != nil {
			reasonCode = *cancelCode
		}

		if err := uc.insertPaymentAttemptCanceledAudit(
			ctx,
			outboxQ,
			input,
			paymentAttemptTerminalAuditFacts{
				InboxID:       inboxID,
				Attempt:       *attempt,
				PreviousState: previousState,
				CurrentState:  model.CanceledPaymentAttemptState,
				ProcessResult: ProcessResultCancellationRecorded,
				ReasonCode:    reasonCode,
				ContractID:    input.ContractID,
				InstallmentID: input.InstallmentID,
			},
			now,
		); err != nil {
			return err
		}

		return uc.finishTerminalInbox(
			ctx,
			inboxQ,
			inboxID,
			ProcessResultCancellationRecorded,
			now,
			output,
		)
	}

	failureCode := "unknown"
	if input.FailureCode != nil && strings.TrimSpace(*input.FailureCode) != "" {
		failureCode = strings.TrimSpace(*input.FailureCode)
	}

	if _, err := paymentAttemptQ.MarkFailed(
		ctx,
		attempt.ID,
		failureCode,
		input.EventOccurredAt.UTC(),
		now,
	); err != nil {
		if cerr.IsCode(err, cerr.CodeConflict) {
			return uc.persistUnmatchedTerminalInstallmentResult(
				ctx,
				inboxQ,
				outboxQ,
				inboxID,
				input,
				"invalid_payment_attempt_state",
				"terminal failure could not be applied to local payment attempt",
				attempt,
				now,
				output,
			)
		}

		return err
	}

	if err := uc.insertPaymentAttemptFailedAudit(
		ctx,
		outboxQ,
		input,
		paymentAttemptTerminalAuditFacts{
			InboxID:       inboxID,
			Attempt:       *attempt,
			PreviousState: previousState,
			CurrentState:  model.FailedPaymentAttemptState,
			ProcessResult: ProcessResultFailureRecorded,
			ReasonCode:    failureCode,
			ContractID:    input.ContractID,
			InstallmentID: input.InstallmentID,
		},
		now,
	); err != nil {
		return err
	}

	return uc.finishTerminalInbox(
		ctx,
		inboxQ,
		inboxID,
		ProcessResultFailureRecorded,
		now,
		output,
	)
}

func (uc *UseCase) persistUnmatchedTerminalInstallmentResult(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	failureCode string,
	failureMessage string,
	attempt *model.PaymentAttempt,
	now time.Time,
	output *ProcessOutput,
) error {
	if _, err := inboxQ.MarkFailed(
		ctx,
		inboxID,
		failureCode,
		failureMessage,
		now,
		now,
	); err != nil {
		return err
	}

	if err := uc.insertUnmatchedTerminalAudit(
		ctx,
		outboxQ,
		input,
		unmatchedTerminalAuditFacts{
			InboxID:       inboxID,
			FailureCode:   failureCode,
			Attempt:       attempt,
			ContractID:    input.ContractID,
			InstallmentID: input.InstallmentID,
		},
		now,
	); err != nil {
		return err
	}

	output.Result = ProcessResultTerminalResultUnmatched

	return nil
}

func (uc *UseCase) finishTerminalInbox(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	inboxID model.InboxID,
	result ProcessResult,
	now time.Time,
	output *ProcessOutput,
) error {
	if _, err := inboxQ.MarkProcessed(ctx, inboxID, now, now); err != nil {
		return err
	}

	output.Result = result

	return nil
}

func debitChargeSourceIDsFromSnapshot(items []model.PaymentAttemptItem) []model.InstallmentChargeID {
	ids := make([]model.InstallmentChargeID, 0)

	for _, item := range items {
		if item.ItemType != model.ChargePaymentAttemptItemType {
			continue
		}

		if item.Effect != model.DebitPaymentAttemptItemEffect {
			continue
		}

		ids = append(ids, model.InstallmentChargeID(item.SourceID))
	}

	return ids
}

func chargeSourceIDsFromSnapshot(items []model.PaymentAttemptItem) []model.InstallmentChargeID {
	ids := make([]model.InstallmentChargeID, 0)

	for _, item := range items {
		if item.ItemType != model.ChargePaymentAttemptItemType {
			continue
		}

		ids = append(ids, model.InstallmentChargeID(item.SourceID))
	}

	return ids
}

func allRequiredInstallmentsPaid(installments []model.Installment) bool {
	if len(installments) == 0 {
		return false
	}

	for _, installment := range installments {
		if installment.Status != model.PaidInstallmentStatus {
			return false
		}
	}

	return true
}

func installmentBelongsToContract(
	installment model.Installment,
	contractID model.ContractID,
) bool {
	return installment.ContractID != nil && *installment.ContractID == contractID
}

func isPayableInstallmentStatus(status model.InstallmentStatus) bool {
	switch status {
	case model.ScheduledInstallmentStatus,
		model.DueInstallmentStatus,
		model.OverdueInstallmentStatus:
		return true

	default:
		return false
	}
}

func installmentAckContext(
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
	attempt *model.PaymentAttempt,
) applicationAckContext {
	ack := applicationAckContext{Purpose: purpose}

	if !input.ContractID.IsZero() {
		contractID := input.ContractID
		ack.ContractID = &contractID
	}

	if !input.InstallmentID.IsZero() {
		installmentID := input.InstallmentID
		ack.InstallmentID = &installmentID
	}

	ack.Attempt = attempt

	return ack
}
