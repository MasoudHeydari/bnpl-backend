package financialcorepaymentresultuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/paymentapplication"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

func (uc *UseCase) processSucceededCheckoutPaymentResult(
	ctx context.Context,
	tx repo.Tx,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxRow model.Inbox,
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
	now time.Time,
	output *ProcessOutput,
) error {
	activateInput := contractuc.ActivateFromFinancialCorePaymentResultInput{
		ContractID:                input.ContractID,
		InvoiceID:                 input.InvoiceID,
		PaymentIntentID:           input.PaymentIntentID,
		FinancialPaymentAttemptID: optionalProviderAttemptID(input.PaymentAttemptID),
		PaymentSessionID:          input.PaymentSessionID,
		PaidAmountMinor:           input.PaidAmountMinor,
		Currency:                  input.Currency,
		PaidAt:                    input.PaidAt,
		EventID:                   input.EventID,
		EventOccurredAt:           input.EventOccurredAt,
		SourceType:                input.SourceType,
		SourceID:                  input.SourceID,
		PaymentStatus:             input.PaymentStatus,
	}

	ack := applicationAckContext{Purpose: purpose}
	if err := validateCheckoutPaymentResultEnvelope(activateInput); err != nil {
		return uc.persistCheckoutPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow.ID,
			input,
			ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			now,
			output,
		)
	}

	if err := validateSucceededCheckoutPaymentMetadata(activateInput); err != nil {
		return uc.persistCheckoutPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow.ID,
			input,
			ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			now,
			output,
		)
	}

	contractID, err := resolveContractID(activateInput)
	if err != nil {
		return uc.persistCheckoutPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow.ID,
			input,
			ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			now,
			output,
		)
	}

	ack.ContractID = &contractID

	contractQ := uc.contract.Tx(tx)
	contractRow, lockErr := contractQ.LockByID(ctx, contractID)
	if lockErr != nil {
		if isContractNotFoundError(lockErr) {
			return uc.persistCheckoutPaymentReconciliation(
				ctx,
				inboxQ,
				outboxQ,
				inboxRow.ID,
				input,
				ack,
				model.ContractNotFoundPaymentReconciliationReason,
				now,
				output,
			)
		}

		return lockErr
	}

	paymentAttemptQ := uc.paymentAttempt.Tx(tx)
	resolvedAttempt, resolveErr := resolvePaymentAttemptForResult(ctx, paymentAttemptQ, input, purpose)
	if resolveErr != nil {
		return uc.persistCheckoutPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow.ID,
			input,
			ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			now,
			output,
		)
	}

	if resolvedAttempt == nil {
		return uc.persistCheckoutPaymentReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow.ID,
			input,
			ack,
			model.UnknownPaymentAttemptPaymentReconciliationReason,
			now,
			output,
		)
	}

	attempt, lockErr := paymentAttemptQ.LockByIDWithItems(ctx, resolvedAttempt.ID)
	if lockErr != nil {
		return lockErr
	}

	ack.Attempt = &attempt

	if err := validateResultTargetsAttempt(attempt, input, purpose); err != nil ||
		attempt.ContractID != contractID ||
		attempt.AmountMinor != input.PaidAmountMinor ||
		attempt.Currency != input.Currency {
		return uc.persistCheckoutAttemptReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxRow.ID,
			input,
			ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			now,
			output,
		)
	}

	if attempt.State == model.FailedPaymentAttemptState || attempt.State == model.CanceledPaymentAttemptState {
		return uc.persistCheckoutAttemptReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxRow.ID,
			input,
			ack,
			model.DuplicateSuccessfulPaymentPaymentReconciliationReason,
			now,
			output,
		)
	}

	enrichedAttempt, enrichErr := enrichAttemptIdentityFromResult(
		ctx,
		paymentAttemptQ,
		attempt,
		input,
		now,
	)
	if enrichErr != nil {
		return uc.persistCheckoutAttemptReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxRow.ID,
			input,
			ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			now,
			output,
		)
	}

	attempt = enrichedAttempt
	ack.Attempt = &attempt

	if contractRow.Status == model.ExpiredContractStatus {
		return uc.persistCheckoutAttemptReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxRow.ID,
			input,
			ack,
			reconciliationReasonForExpiredContract(contractRow, input.PaidAt),
			now,
			output,
		)
	}

	if input.PaidAmountMinor != contractRow.PayableNowAmountMinor {
		return uc.persistCheckoutAttemptReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxRow.ID,
			input,
			ack,
			model.PaymentAmountMismatchPaymentReconciliationReason,
			now,
			output,
		)
	}

	activateOutput, activateErr := uc.contractUC.ApplyCheckoutPaymentActivationInTx(
		ctx,
		tx,
		activateInput,
		contractRow,
		now,
	)
	if activateErr != nil {
		if isCheckoutActivationReconciliationError(activateErr) {
			return uc.persistCheckoutAttemptReconciliation(
				ctx,
				inboxQ,
				outboxQ,
				paymentAttemptQ,
				inboxRow.ID,
				input,
				ack,
				reconciliationReasonFromError(activateErr),
				now,
				output,
			)
		}

		return activateErr
	}

	result := paymentapplication.AppliedApplicationResult
	var appliedAt *time.Time
	if activateOutput.Result == contractuc.ActivateResultAlreadyActive {
		result = paymentapplication.AlreadyAppliedApplicationResult
		appliedAt = originalApplicationTimestamp(nil, &activateOutput.FirstInstallment, &activateOutput.Contract)
	} else {
		value := now.UTC()
		appliedAt = &value
	}

	if attempt.State != model.SucceededPaymentAttemptState {
		markAppliedAt := now.UTC()
		if appliedAt != nil {
			markAppliedAt = appliedAt.UTC()
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
			return markErr
		}

		attempt = succeededAttempt
	} else if !duplicateSuccessIdentityMatches(attempt, input) {
		return uc.persistCheckoutAttemptReconciliation(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxRow.ID,
			input,
			ack,
			model.DuplicateSuccessfulPaymentPaymentReconciliationReason,
			now,
			output,
		)
	}

	contractStatus := activateOutput.Contract.Status
	installmentID := activateOutput.FirstInstallment.ID
	installmentStatus := activateOutput.FirstInstallment.Status
	ack.Attempt = &attempt
	ack.ContractStatus = &contractStatus
	ack.InstallmentID = &installmentID
	ack.InstallmentStatus = &installmentStatus

	if activateOutput.Result == contractuc.ActivateResultActivated {
		appliedAtValue := now.UTC()
		if appliedAt != nil {
			appliedAtValue = appliedAt.UTC()
		}

		if err := uc.insertCheckoutActivationAudit(
			ctx,
			outboxQ,
			input,
			checkoutActivationAuditFacts{
				InboxID:        inboxRow.ID,
				ActivateOutput: activateOutput,
				Attempt:        attempt,
				AppliedAt:      appliedAtValue,
			},
			now,
		); err != nil {
			return err
		}
	}

	// Advance the linked CheckoutSession only after contract activation committed
	// in this transaction. Duplicate success events are idempotent: completed
	// never regresses.
	if err := uc.checkoutSessionUC.ApplyPaymentSucceeded(ctx, tx, contractID, now); err != nil {
		return err
	}

	return uc.finishSuccessfulPaymentApplication(
		ctx,
		inboxQ,
		outboxQ,
		inboxRow.ID,
		input,
		ack,
		result,
		appliedAt,
		now,
		output,
	)
}

func (uc *UseCase) persistCheckoutAttemptReconciliation(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	ack applicationAckContext,
	reason model.PaymentReconciliationReason,
	now time.Time,
	output *ProcessOutput,
) error {
	var (
		attemptChanged bool
		previousState  model.PaymentAttemptState
	)
	const op = "financialcorepaymentresultuc.UseCase.persistCheckoutAttemptReconciliation"

	if ack.Attempt != nil {
		previousState = ack.Attempt.State
		changed, err := uc.transitionAttemptToReconciliation(
			ctx,
			paymentAttemptQ,
			ack.Attempt.ID,
			input,
			reason,
			now,
		)
		if err != nil {
			return err
		}

		attemptChanged = changed
	}

	if input.PaymentIntentID == "" {
		return fmt.Errorf("%s: financial payment intent id is required for reconciliation acknowledgement", op)
	}

	if _, err := inboxQ.MarkReconciliationRequired(
		ctx,
		inboxID,
		stringPtr(string(reason)),
		nil,
		now,
		now,
	); err != nil {
		return err
	}

	event := buildApplicationEvent(
		uc.newPaymentApplicationEventID(),
		input,
		ack,
		paymentapplication.ReconciliationRequiredApplicationResult,
		&reason,
		nil,
		now,
	)

	if err := uc.insertPaymentApplicationOutbox(ctx, outboxQ, event, now); err != nil {
		return err
	}

	facts := reconciliationAuditFacts{
		InboxID:             inboxID,
		Reason:              reason,
		Attempt:             ack.Attempt,
		AttemptStateChanged: attemptChanged,
	}
	if ack.ContractID != nil {
		facts.ContractID = *ack.ContractID
	}

	if attemptChanged {
		facts.PreviousPaymentAttemptState = &previousState
		current := model.ReconciliationRequiredPaymentAttemptState
		facts.CurrentPaymentAttemptState = &current
	}

	if err := uc.insertReconciliationAudit(
		ctx,
		outboxQ,
		input,
		facts,
		now,
	); err != nil {
		return err
	}

	output.Result = ProcessResultReconciliationRequired
	return nil
}

func reconciliationReasonForExpiredContract(
	contractRow model.Contract,
	paidAt time.Time,
) model.PaymentReconciliationReason {
	if contractRow.PaymentOccurredOnTime(paidAt) {
		return model.ContractExpiredPaymentWasOnTimePaymentReconciliationReason
	}

	return model.PaymentAfterDeadlinePaymentReconciliationReason
}

func resolveCheckoutInboxSubject(input ProcessInput) (subjectType *string, subjectID *string) {
	activateInput := contractuc.ActivateFromFinancialCorePaymentResultInput{
		ContractID: input.ContractID,
		SourceType: input.SourceType,
		SourceID:   input.SourceID,
	}

	contractID, err := resolveContractID(activateInput)
	if err == nil && !contractID.IsZero() {
		st := string(model.ContractInboxSubjectType)
		sid := contractID.String()

		return &st, &sid
	}

	if input.PaymentIntentID != "" {
		st := string(model.FinancialPaymentIntentInboxSubjectType)
		sid := input.PaymentIntentID

		return &st, &sid
	}

	return nil, nil
}

func unsupportedPurposeAck(input ProcessInput, purpose model.FinancialCorePaymentPurpose) applicationAckContext {
	ack := applicationAckContext{Purpose: purpose}

	if !input.ContractID.IsZero() {
		contractID := input.ContractID
		ack.ContractID = &contractID
	}

	if !input.InstallmentID.IsZero() {
		installmentID := input.InstallmentID
		ack.InstallmentID = &installmentID
	}

	return ack
}
