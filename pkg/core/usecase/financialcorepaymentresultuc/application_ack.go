package financialcorepaymentresultuc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/paymentapplication"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

type applicationAckContext struct {
	Purpose           model.FinancialCorePaymentPurpose
	ContractID        *model.ContractID
	InstallmentID     *model.InstallmentID
	Attempt           *model.PaymentAttempt
	ContractStatus    *model.ContractStatus
	InstallmentStatus *model.InstallmentStatus
}

func (uc *UseCase) insertPaymentApplicationOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	event paymentapplication.Event,
	now time.Time,
) error {
	const op = "financialcorepaymentresultuc.UseCase.insertPaymentApplicationOutbox"

	if uc.paymentApplicationResultsDestination == "" {
		return fmt.Errorf("%s: payment application results destination is not configured", op)
	}

	if err := paymentapplication.ValidateEvent(event); err != nil {
		return fmt.Errorf("%s: validate payment application result event: %w", op, err)
	}

	raw, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal payment application result payload: %w", op, err)
	}

	aggregateType := "payment_application"
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.paymentApplicationResultsDestination,
		MessageKey:    event.Data.FinancialPaymentIntentID,
		EventType:     paymentapplication.PaymentApplicationResultEventType,
		AggregateType: &aggregateType,
		Payload:       raw,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert payment application result outbox message: %w", op, err)
	}

	return nil
}

func (uc *UseCase) newPaymentApplicationEventID() string {
	return uc.idGenerator.Generate()
}

func buildApplicationEvent(
	eventID string,
	input ProcessInput,
	ack applicationAckContext,
	result paymentapplication.ApplicationResult,
	reason *model.PaymentReconciliationReason,
	appliedAt *time.Time,
	occurredAt time.Time,
) paymentapplication.Event {
	paidAmount, currency, financialPaidAt := paymentEvidenceFromInput(input)

	data := paymentapplication.EventData{
		FinancialPaymentIntentID: input.PaymentIntentID,
		PaymentPurpose:           ack.Purpose,
		Result:                   result,
		PaidAmountMinor:          paidAmount,
		Currency:                 currency,
		FinancialPaidAt:          financialPaidAt,
		BNPLAppliedAt:            appliedAt,
		ContractStatus:           ack.ContractStatus,
		InstallmentStatus:        ack.InstallmentStatus,
	}

	if providerAttemptID := optionalProviderAttemptID(input.PaymentAttemptID); providerAttemptID != nil {
		data.FinancialPaymentAttemptID = providerAttemptID
	}

	if reason != nil {
		data.Reason = reason
	}

	if ack.ContractID != nil {
		data.ContractID = ack.ContractID
	}

	if ack.InstallmentID != nil {
		data.InstallmentID = ack.InstallmentID
	}

	if ack.Attempt != nil {
		data.BNPLPaymentAttemptID = &ack.Attempt.ID
	}

	return paymentapplication.NewEvent(
		eventID,
		input.EventID,
		input.CorrelationID,
		occurredAt,
		data,
	)
}

func paymentEvidenceFromInput(input ProcessInput) (*uint64, *model.Currency, *time.Time) {
	var (
		paidAmount *uint64
		currency   *model.Currency
		paidAt     *time.Time
	)

	if input.PaidAmountMinor > 0 {
		value := input.PaidAmountMinor
		paidAmount = &value
	}

	if input.Currency != "" {
		value := input.Currency
		currency = &value
	}

	if !input.PaidAt.IsZero() {
		value := input.PaidAt.UTC()
		paidAt = &value
	}

	return paidAmount, currency, paidAt
}

func (uc *UseCase) persistCheckoutPaymentReconciliation(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	ack applicationAckContext,
	reason model.PaymentReconciliationReason,
	now time.Time,
	output *ProcessOutput,
) error {
	const op = "financialcorepaymentresultuc.UseCase.persistCheckoutPaymentReconciliation"

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
		InboxID: inboxID,
		Reason:  reason,
		Attempt: ack.Attempt,
	}
	if ack.ContractID != nil {
		facts.ContractID = *ack.ContractID
	}

	if ack.InstallmentID != nil {
		facts.InstallmentID = *ack.InstallmentID
	}

	if err := uc.insertReconciliationAudit(ctx, outboxQ, input, facts, now); err != nil {
		return err
	}

	output.Result = ProcessResultReconciliationRequired

	return nil
}

func (uc *UseCase) persistInstallmentPaymentReconciliation(
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
	const op = "financialcorepaymentresultuc.UseCase.persistInstallmentPaymentReconciliation"

	if input.PaymentIntentID == "" {
		return fmt.Errorf("%s: financial payment intent id is required for reconciliation acknowledgement", op)
	}

	facts := reconciliationAuditFacts{
		InboxID: inboxID,
		Reason:  reason,
		Attempt: ack.Attempt,
	}
	if ack.ContractID != nil {
		facts.ContractID = *ack.ContractID
	}

	if ack.InstallmentID != nil {
		facts.InstallmentID = *ack.InstallmentID
	}

	if ack.Attempt != nil {
		previousState := ack.Attempt.State
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

		if changed {
			facts.AttemptStateChanged = true
			facts.PreviousPaymentAttemptState = &previousState
			current := model.ReconciliationRequiredPaymentAttemptState
			facts.CurrentPaymentAttemptState = &current
		}
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

	if err := uc.insertReconciliationAudit(ctx, outboxQ, input, facts, now); err != nil {
		return err
	}

	output.Result = ProcessResultReconciliationRequired

	return nil
}

// transitionAttemptToReconciliation moves a locally locked payment attempt to
// reconciliation_required when it is still submitted. It re-locks the attempt to decide
// from authoritative state and never silently swallows a transition conflict.
// The bool return is true when the attempt state was newly changed.
func (uc *UseCase) transitionAttemptToReconciliation(
	ctx context.Context,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	attemptID model.PaymentAttemptID,
	input ProcessInput,
	reason model.PaymentReconciliationReason,
	now time.Time,
) (bool, error) {
	const op = "financialcorepaymentresultuc.UseCase.transitionAttemptToReconciliation"

	locked, err := paymentAttemptQ.LockByID(ctx, attemptID)
	if err != nil {
		return false, err
	}

	switch locked.State {
	case model.CreatedPaymentAttemptState, model.SubmittedPaymentAttemptState:
		var paidAt *time.Time
		if !input.PaidAt.IsZero() {
			value := input.PaidAt.UTC()
			paidAt = &value
		}

		// The row is locked in this transaction; a conflict here is unexpected and must
		// not be ignored. Returning the error rolls back the whole transaction.
		if _, markErr := paymentAttemptQ.MarkReconciliationRequired(
			ctx,
			locked.ID,
			reason,
			optionalProviderAttemptID(input.PaymentAttemptID),
			paidAt,
			now,
		); markErr != nil {
			return false, markErr
		}

		return true, nil

	case model.ReconciliationRequiredPaymentAttemptState:
		// Already reconciled: idempotent no-op. The inbox and outbox reconciliation records
		// still capture the new inbound successful event and preserve the original timestamp.
		return false, nil

	case model.SucceededPaymentAttemptState,
		model.FailedPaymentAttemptState,
		model.CanceledPaymentAttemptState,
		model.ManualReviewRequiredPaymentAttemptState:
		// Terminal states must not be overwritten. already_applied versus different payment
		// identity is decided before this function is called; the durable inbox and outbox
		// reconciliation still records the new inbound successful event.
		return false, nil

	default:
		return false, cerr.InternalError(fmt.Errorf(
			"%s: unhandled payment attempt state %q for reconciliation", op, locked.State,
		))
	}
}

func (uc *UseCase) finishSuccessfulPaymentApplication(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	ack applicationAckContext,
	result paymentapplication.ApplicationResult,
	appliedAt *time.Time,
	now time.Time,
	output *ProcessOutput,
) error {
	if _, err := inboxQ.MarkProcessed(ctx, inboxID, now, now); err != nil {
		return err
	}

	event := buildApplicationEvent(
		uc.newPaymentApplicationEventID(),
		input,
		ack,
		result,
		nil,
		appliedAt,
		now,
	)

	if err := uc.insertPaymentApplicationOutbox(ctx, outboxQ, event, now); err != nil {
		return err
	}

	switch result {
	case paymentapplication.AlreadyAppliedApplicationResult:
		output.Result = ProcessResultAlreadyApplied

	default:
		output.Result = ProcessResultPaymentApplied
	}

	return nil
}

func originalApplicationTimestamp(
	attempt *model.PaymentAttempt,
	installment *model.Installment,
	contract *model.Contract,
) *time.Time {
	if attempt != nil && attempt.AppliedAt != nil {
		value := attempt.AppliedAt.UTC()
		return &value
	}

	if installment != nil && installment.PaidAt != nil {
		value := installment.PaidAt.UTC()
		return &value
	}

	if contract != nil && contract.ActivatedAt != nil {
		value := contract.ActivatedAt.UTC()
		return &value
	}

	return nil
}

func reconciliationReasonFromError(err error) model.PaymentReconciliationReason {
	var reconErr *contractuc.ReconciliationError
	if errors.As(err, &reconErr) {
		return reconErr.Reason
	}

	if errors.Is(err, contractuc.ErrPaymentRequiresReconciliation) {
		return model.ContractExpiredPaymentWasOnTimePaymentReconciliationReason
	}

	return model.PaymentReferenceMismatchPaymentReconciliationReason
}

func isCheckoutActivationReconciliationError(err error) bool {
	if err == nil {
		return false
	}

	var reconErr *contractuc.ReconciliationError
	if errors.As(err, &reconErr) {
		return true
	}

	return errors.Is(err, contractuc.ErrPaymentRequiresReconciliation)
}
