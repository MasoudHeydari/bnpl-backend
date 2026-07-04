package financialcorepaymentreconciliationuc

import (
	"context"
	"errors"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

func (uc *UseCase) handleStaleGenerationSucceeded(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	contractQ repo.ContractTxQueryer,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	accountQ repo.CreditAccountTxQueryer,
	inboxID model.InboxID,
	inboxRow model.Inbox,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	now time.Time,
	output *ProcessOutput,
) error {
	if attempt.State == model.SucceededPaymentAttemptState {
		if err := validateSucceededResultAgainstAttempt(input, attempt); err != nil {
			reason := model.PaymentReferenceMismatchPaymentReconciliationReason
			return uc.persistPermanentInboxFailureWithAcknowledgement(
				ctx,
				inboxQ,
				outboxQ,
				inboxRow,
				input,
				attempt,
				previousState,
				"stale_succeeded_replay_conflict",
				err.Error(),
				&reason,
				now,
				output,
			)
		}

		return uc.finishAlreadyProcessed(
			ctx,
			inboxQ,
			outboxQ,
			inboxID,
			input,
			attempt,
			previousState,
			now,
			output,
		)
	}

	if err := validateStaleGenerationSucceededInput(input, attempt); err != nil {
		reason := model.PaymentReferenceMismatchPaymentReconciliationReason
		return uc.persistPermanentInboxFailureWithAcknowledgement(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow,
			input,
			attempt,
			previousState,
			"stale_reconciliation_success_invalid",
			err.Error(),
			&reason,
			now,
			output,
		)
	}

	paidAt := now
	if input.Data.PaidAt != nil {
		paidAt = input.Data.PaidAt.UTC()
	}

	paymentInput := financialcorepaymentresultuc.BuildReconciliationPaymentInput(
		input.EventID,
		input.EventType,
		input.EventVersion,
		input.SourceService,
		input.CorrelationID,
		input.CausationID,
		input.RawPayload,
		attempt.ContractID,
		attempt.InstallmentID,
		stringValue(input.Data.FinancialPaymentIntentID),
		stringValue(input.Data.FinancialPaymentAttemptID),
		stringValue(input.Data.FinancialPaymentSessionID),
		input.Data.AmountMinor,
		attempt.Currency,
		paidAt,
		input.Data.OccurredAt.UTC(),
	)

	var paymentOutput financialcorepaymentresultuc.ProcessOutput
	if err := uc.paymentResult.ApplyReconciliationSucceededInstallmentPayment(
		ctx,
		inboxQ,
		outboxQ,
		paymentAttemptQ,
		contractQ,
		installmentQ,
		installmentChargeQ,
		accountQ,
		inboxID,
		paymentInput,
		attempt,
		now,
		&paymentOutput,
	); err != nil {
		return err
	}

	updatedAttempt, err := paymentAttemptQ.LockByID(ctx, attempt.ID)
	if err != nil {
		return err
	}

	if err := uc.insertResolutionAudit(
		ctx,
		outboxQ,
		input,
		updatedAttempt,
		previousState,
		auditActionResolveSucceeded,
		auditReasonResolvedSucceeded,
		false,
		now,
	); err != nil {
		return err
	}

	if err := uc.insertReconciliationAcknowledgement(
		ctx,
		outboxQ,
		input,
		updatedAttempt,
		previousState,
		updatedAttempt.State,
		installmentpaymentreconciliation.ResolvedSucceededAcknowledgementResult,
		updatedAttempt.FinancialPaymentIntentID,
		updatedAttempt.ReconciliationReason,
		now,
	); err != nil {
		return err
	}

	output.Result = ProcessResultResolvedSucceeded

	return nil
}

func validateStaleGenerationSucceededInput(
	input ProcessInput,
	attempt model.PaymentAttempt,
) error {
	if input.Data.BNPLPaymentAttemptID != attempt.ID {
		return errors.New("stale reconciliation success payment attempt id mismatch")
	}

	if stringValue(input.Data.FinancialPaymentIntentID) == "" {
		return errors.New("stale reconciliation success requires financial payment intent id")
	}

	if input.Data.AmountMinor != attempt.AmountMinor {
		return errors.New("stale reconciliation success amount mismatch")
	}

	if model.Currency(input.Data.Currency) != attempt.Currency {
		return errors.New("stale reconciliation success currency mismatch")
	}

	if intent := stringValue(input.Data.FinancialPaymentIntentID); intent != "" &&
		attempt.FinancialPaymentIntentID != nil &&
		*attempt.FinancialPaymentIntentID != "" &&
		*attempt.FinancialPaymentIntentID != intent {
		return errors.New("stale reconciliation success payment intent conflict")
	}

	return nil
}
