package financialcorepaymentresultuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/paymentapplication"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ApplyReconciliationSucceededInstallmentPayment applies an authoritative Financial Core
// success while a payment attempt is reconciliation_required.
func (uc *UseCase) ApplyReconciliationSucceededInstallmentPayment(
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
	attempt model.PaymentAttempt,
	now time.Time,
	output *ProcessOutput,
) error {
	const op = "financialcorepaymentresultuc.UseCase.ApplyReconciliationSucceededInstallmentPayment"

	purpose := model.InstallmentRepaymentFinancialCorePaymentPurpose
	ack := installmentAckContext(input, purpose, &attempt)

	if !canAcceptReconciliationSuccess(attempt.State) {
		return newReconciliationApplicationError(
			model.InvalidPaymentAttemptStatePaymentReconciliationReason,
			fmt.Errorf("%s: payment attempt state %q cannot accept reconciliation success", op, attempt.State),
		)
	}

	if input.PaymentIntentID == "" {
		return newReconciliationApplicationError(
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: financial core payment intent is required", op),
		)
	}

	lockedItems := attempt.Items
	enrichedAttempt, enrichErr := paymentAttemptQ.EnrichFinancialPaymentIdentityIfAbsent(
		ctx,
		attempt.ID,
		input.PaymentIntentID,
		optionalPaymentSessionID(input.PaymentSessionID),
		optionalProviderAttemptID(input.PaymentAttemptID),
		now,
	)
	if enrichErr != nil {
		if cerr.IsCode(enrichErr, cerr.CodeConflict) ||
			cerr.IsCode(enrichErr, cerr.CodeValidationFailed) ||
			cerr.IsCode(enrichErr, cerr.CodeIdempotencyConflict) {
			return newReconciliationApplicationError(
				model.PaymentReferenceMismatchPaymentReconciliationReason,
				enrichErr,
			)
		}

		return enrichErr
	}

	// The enrichment update returns the payment-attempt parent row only. Preserve
	// the immutable item snapshot loaded and locked by LockByIDWithItems; never
	// rebuild it from current installment or charge rows.
	enrichedAttempt.Items = lockedItems
	attempt = enrichedAttempt

	if err := validateFinancialPaymentIntentMatchesAttempt(attempt, input); err != nil {
		return newReconciliationApplicationError(
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			err,
		)
	}

	contractRow, err := contractQ.LockByID(ctx, input.ContractID)
	if err != nil {
		return err
	}

	installmentRow, err := installmentQ.LockByID(ctx, input.InstallmentID)
	if err != nil {
		return err
	}

	payableAmount, snapshotOK := model.PayableAmountFromItems(attempt.Items)
	if !snapshotOK || payableAmount == 0 || attempt.AmountMinor != payableAmount {
		return newReconciliationApplicationError(
			model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
			fmt.Errorf("%s: payment attempt snapshot is inconsistent", op),
		)
	}

	if !installmentBelongsToContract(installmentRow, input.ContractID) {
		return newReconciliationApplicationError(
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: installment does not belong to contract", op),
		)
	}

	if installmentRow.Status == model.PaidInstallmentStatus {
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
		return newReconciliationApplicationError(
			*validation.reason,
			fmt.Errorf("%s: installment success preconditions failed: %s", op, *validation.reason),
		)
	}

	lockedCharges, err := installmentChargeQ.LockByIDs(ctx, chargeSourceIDsFromSnapshot(attempt.Items))
	if err != nil {
		return err
	}

	creditAccount, err := lockCreditAccountForPrincipalRelease(
		ctx, accountQ, contractRow, installmentRow.PrincipalAmountMinor,
	)
	if err != nil {
		return err
	}

	if reason, ok := validatePaymentAttemptSnapshot(attempt, installmentRow, lockedCharges, payableAmount); !ok {
		return newReconciliationApplicationError(
			reason,
			fmt.Errorf("%s: payment attempt snapshot validation failed: %s", op, reason),
		)
	}

	if contractRow.RemainingPrincipalAmountMinor < installmentRow.PrincipalAmountMinor {
		return newReconciliationApplicationError(
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: contract remaining principal is insufficient", op),
		)
	}

	return uc.applyNewInstallmentPaymentFromReconciliation(
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

func (uc *UseCase) applyNewInstallmentPaymentFromReconciliation(
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
	const op = "financialcorepaymentresultuc.UseCase.applyNewInstallmentPaymentFromReconciliation"

	succeededAttempt, err := markAttemptSucceededFromReconciliation(
		ctx,
		paymentAttemptQ,
		attempt,
		optionalProviderAttemptID(input.PaymentAttemptID),
		input.PaidAt.UTC(),
		now,
	)
	if err != nil {
		return wrapDeterministicReconciliationApplicationError(
			model.InvalidPaymentAttemptStatePaymentReconciliationReason,
			err,
		)
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
		return wrapDeterministicReconciliationApplicationError(
			model.InvalidInstallmentStatePaymentReconciliationReason,
			err,
		)
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
			return wrapDeterministicReconciliationApplicationError(
				model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
				markChargeErr,
			)
		}

		if affected != int64(len(debitChargeIDs)) {
			return newReconciliationApplicationError(
				model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason,
				fmt.Errorf("%s: snapshotted charge settlement affected row count mismatch", op),
			)
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
		return wrapDeterministicReconciliationApplicationError(
			model.ExposureInconsistencyPaymentReconciliationReason, err,
		)
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
			return wrapDeterministicReconciliationApplicationError(
				model.PaymentReferenceMismatchPaymentReconciliationReason,
				err,
			)
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
			return newReconciliationApplicationError(
				model.InvalidContractStatePaymentReconciliationReason,
				err,
			)
		}

		completedContract, completeMarkErr := contractQ.MarkCompleted(ctx, contractRow.ID, now, now)
		if completeMarkErr != nil {
			return wrapDeterministicReconciliationApplicationError(
				model.InvalidContractStatePaymentReconciliationReason,
				completeMarkErr,
			)
		}

		finalContractStatus = completedContract.Status
	}

	ack.Attempt = &succeededAttempt
	installmentStatus := paidInstallment.Status
	ack.InstallmentStatus = &installmentStatus
	ack.ContractStatus = &finalContractStatus
	appliedAtValue := now.UTC()

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

func canAcceptReconciliationSuccess(state model.PaymentAttemptState) bool {
	switch state {
	case model.ReconciliationRequiredPaymentAttemptState, model.SubmittedPaymentAttemptState:
		return true

	default:
		return false
	}
}

func markAttemptSucceededFromReconciliation(
	ctx context.Context,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	attempt model.PaymentAttempt,
	financialPaymentAttemptID *string,
	paidAt time.Time,
	now time.Time,
) (model.PaymentAttempt, error) {
	const op = "financialcorepaymentresultuc.markAttemptSucceededFromReconciliation"

	switch attempt.State {
	case model.ReconciliationRequiredPaymentAttemptState:
		return paymentAttemptQ.MarkSucceededFromReconciliationRequired(
			ctx,
			attempt.ID,
			financialPaymentAttemptID,
			paidAt,
			now,
			now,
			now,
		)

	case model.SubmittedPaymentAttemptState:
		return paymentAttemptQ.MarkSucceeded(
			ctx,
			attempt.ID,
			financialPaymentAttemptID,
			paidAt,
			now,
			now,
		)

	default:
		return model.PaymentAttempt{}, newReconciliationApplicationError(
			model.InvalidPaymentAttemptStatePaymentReconciliationReason,
			fmt.Errorf("%s: payment attempt state %q cannot accept reconciliation success", op, attempt.State),
		)
	}
}

func optionalPaymentSessionID(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

// BuildReconciliationPaymentInput maps a reconciliation result to payment-result processing input.
func BuildReconciliationPaymentInput(
	eventID string,
	eventType string,
	eventVersion string,
	sourceService string,
	correlationID *string,
	causationID *string,
	rawPayload []byte,
	contractID model.ContractID,
	installmentID model.InstallmentID,
	paymentIntentID string,
	paymentAttemptID string,
	paymentSessionID string,
	paidAmountMinor uint64,
	currency model.Currency,
	paidAt time.Time,
	occurredAt time.Time,
) ProcessInput {
	return ProcessInput{
		EventID:          eventID,
		EventType:        eventType,
		EventVersion:     eventVersion,
		SourceService:    sourceService,
		CorrelationID:    correlationID,
		CausationID:      causationID,
		RawPayload:       rawPayload,
		PaymentPurpose:   model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ContractID:       contractID,
		InstallmentID:    installmentID,
		PaymentIntentID:  paymentIntentID,
		PaymentAttemptID: paymentAttemptID,
		PaymentSessionID: paymentSessionID,
		PaymentStatus:    model.SucceededFinancialPaymentResultStatus,
		PaidAmountMinor:  paidAmountMinor,
		Currency:         currency,
		PaidAt:           paidAt,
		EventOccurredAt:  occurredAt,
	}
}
