package financialcorepaymentreconciliationuc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

// ProcessInput carries a Financial Core reconciliation result with inbox metadata.
type ProcessInput struct {
	// EventID uniquely identifies the inbound event.
	EventID string
	// EventType is the inbound event type.
	EventType string
	// EventVersion is the inbound event schema version.
	EventVersion string
	// SourceService identifies the producing service.
	SourceService string

	// CorrelationID correlates related events across services.
	CorrelationID *string
	// CausationID identifies the causing event or request.
	CausationID *string

	// RawPayload is the original event payload bytes.
	RawPayload json.RawMessage

	// Data carries opaque provider or reconciliation payload data.
	Data installmentpaymentreconciliation.ResultEventData
}

// ProcessOutput reports how a reconciliation result was handled.
type ProcessOutput struct {
	// Result reports the use-case outcome classification.
	Result ProcessResult
}

// ProcessResult identifies durable reconciliation processing outcomes.
type ProcessResult string

// Known durable reconciliation processing outcomes.
const (
	ProcessResultResolvedSubmitted           ProcessResult = "resolved_submitted"
	ProcessResultResolvedSucceeded           ProcessResult = "resolved_succeeded"
	ProcessResultResolvedFailed              ProcessResult = "resolved_failed"
	ProcessResultResolvedCanceled            ProcessResult = "resolved_canceled"
	ProcessResultStillReconciliationRequired ProcessResult = "still_reconciliation_required"
	ProcessResultAlreadyProcessed            ProcessResult = "already_processed"
	ProcessResultPayloadConflictRecorded     ProcessResult = "payload_conflict_recorded"
	ProcessResultPermanentlyRejected         ProcessResult = "permanently_rejected"
	ProcessResultStaleSuperseded             ProcessResult = "stale_superseded"
)

// Process applies a Financial Core reconciliation result durably.
func (uc *UseCase) Process(ctx context.Context, input ProcessInput) (ProcessOutput, error) {
	const op = "financialcorepaymentreconciliationuc.UseCase.Process"

	input = normalizeProcessInput(input)

	if err := validateMinimumTrustedEnvelope(input); err != nil {
		return ProcessOutput{}, err
	}

	now := uc.clock.Now().UTC()
	var output ProcessOutput

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			if uc.paymentResult != nil {
				if err := uc.paymentResult.LockSettlementAttemptOnTx(ctx, tx, input.Data.BNPLPaymentAttemptID); err != nil {
					return err
				}
			}
			inboxQ := uc.inbox.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)
			contractQ := uc.contract.Tx(tx)
			installmentQ := uc.installment.Tx(tx)
			installmentChargeQ := uc.installmentCharge.Tx(tx)
			accountQ := uc.creditAccount.Tx(tx)

			subjectType, subjectID := inboxSubjectForPaymentAttempt(input.Data.BNPLPaymentAttemptID)
			acquire, acquireErr := uc.acquireInboxEvent(
				ctx,
				inboxQ,
				input,
				subjectType,
				subjectID,
				now,
			)
			if acquireErr != nil {
				return acquireErr
			}

			if acquire.Result == model.InboxAcquirePayloadMismatch {
				return uc.recordPayloadIdentityConflict(
					ctx,
					inboxQ,
					acquire.Row,
					input,
					now,
					&output,
				)
			}

			if mapInboxAcquireResult(acquire.Result, &output) {
				return nil
			}

			if validationErr := installmentpaymentreconciliation.ValidateResultEventData(input.Data); validationErr != nil {
				return uc.persistPermanentInboxFailure(
					ctx,
					inboxQ,
					acquire.Row,
					"invalid_reconciliation_result",
					validationErr.Error(),
					now,
					&output,
				)
			}

			attempt, lockErr := paymentAttemptQ.LockByIDWithItems(ctx, input.Data.BNPLPaymentAttemptID)
			if lockErr != nil {
				if cerr.IsCode(lockErr, cerr.CodeResourceNotFound) {
					return uc.persistPermanentInboxFailure(
						ctx,
						inboxQ,
						acquire.Row,
						"unknown_payment_attempt",
						"bnpl payment attempt was not found",
						now,
						&output,
					)
				}

				return lockErr
			}

			previousState := attempt.State
			inboxID := acquire.Row.ID

			if err := validateReconciliationRequestIdentity(input, attempt); err != nil {
				reason := model.PaymentReferenceMismatchPaymentReconciliationReason
				return uc.persistPermanentInboxFailureWithAcknowledgement(
					ctx,
					inboxQ,
					outboxQ,
					acquire.Row,
					input,
					attempt,
					previousState,
					"request_event_id_mismatch",
					err.Error(),
					&reason,
					now,
					&output,
				)
			}

			if input.Data.ReconciliationGeneration != attempt.ReconciliationAttemptCount {
				if input.Data.ReconciliationGeneration > attempt.ReconciliationAttemptCount {
					reason := model.InvalidPaymentAttemptStatePaymentReconciliationReason
					return uc.persistPermanentInboxFailureWithAcknowledgement(
						ctx,
						inboxQ,
						outboxQ,
						acquire.Row,
						input,
						attempt,
						previousState,
						"future_reconciliation_generation",
						"future reconciliation generation cannot apply payment",
						&reason,
						now,
						&output,
					)
				}

				if input.Data.Status == installmentpaymentreconciliation.SucceededResultStatus {
					return uc.handleStaleGenerationSucceeded(
						ctx,
						inboxQ,
						outboxQ,
						paymentAttemptQ,
						contractQ,
						installmentQ,
						installmentChargeQ,
						accountQ,
						inboxID,
						acquire.Row,
						input,
						attempt,
						previousState,
						now,
						&output,
					)
				}

				if attempt.State == model.SucceededPaymentAttemptState {
					return uc.finishAlreadyProcessed(
						ctx,
						inboxQ,
						outboxQ,
						inboxID,
						input,
						attempt,
						previousState,
						now,
						&output,
					)
				}

				return uc.finishStaleSuperseded(
					ctx,
					inboxQ,
					outboxQ,
					inboxID,
					input,
					attempt,
					previousState,
					now,
					&output,
				)
			}

			switch input.Data.Status {
			case installmentpaymentreconciliation.PendingResultStatus:
				return uc.handlePending(
					ctx,
					inboxQ,
					outboxQ,
					paymentAttemptQ,
					contractQ,
					inboxID,
					acquire.Row,
					input,
					attempt,
					previousState,
					now,
					&output,
				)

			case installmentpaymentreconciliation.SucceededResultStatus:
				return uc.handleSucceeded(
					ctx,
					inboxQ,
					outboxQ,
					paymentAttemptQ,
					contractQ,
					installmentQ,
					installmentChargeQ,
					accountQ,
					inboxID,
					acquire.Row,
					input,
					attempt,
					previousState,
					now,
					&output,
				)

			case installmentpaymentreconciliation.FailedResultStatus:
				return uc.handleFailed(
					ctx,
					inboxQ,
					outboxQ,
					paymentAttemptQ,
					inboxID,
					input,
					attempt,
					previousState,
					now,
					&output,
				)

			case installmentpaymentreconciliation.CanceledResultStatus:
				return uc.handleCanceled(
					ctx,
					inboxQ,
					outboxQ,
					paymentAttemptQ,
					contractQ,
					installmentQ,
					installmentChargeQ,
					inboxID,
					input,
					attempt,
					previousState,
					now,
					&output,
				)

			case installmentpaymentreconciliation.NotFoundResultStatus:
				return uc.handleNotFound(
					ctx,
					inboxQ,
					outboxQ,
					paymentAttemptQ,
					contractQ,
					installmentQ,
					installmentChargeQ,
					inboxID,
					input,
					attempt,
					previousState,
					now,
					&output,
				)

			default:
				return unsupportedReconciliationResultStatusError(fmt.Errorf("%s: unsupported reconciliation result status", op))
			}
		})
	})
	if err != nil {
		if applicationErr, ok := financialcorepaymentresultuc.AsReconciliationApplicationError(err); ok {
			return uc.persistReconciliationApplicationConflict(ctx, input, applicationErr, now)
		}

		return ProcessOutput{}, err
	}

	if uc.paymentResult != nil {
		if err := uc.paymentResult.ReassessSettledAttempt(ctx, input.Data.BNPLPaymentAttemptID); err != nil {
			return output, err
		}
	}
	return output, nil
}

func (uc *UseCase) handlePending(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	contractQ repo.ContractTxQueryer,
	inboxID model.InboxID,
	inboxRow model.Inbox,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	now time.Time,
	output *ProcessOutput,
) error {
	if attempt.State != model.ReconciliationRequiredPaymentAttemptState {
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

	if err := pendingResultEconomicsConflict(input, attempt); err != nil {
		reason := model.PaymentReferenceMismatchPaymentReconciliationReason
		return uc.persistPermanentInboxFailureWithAcknowledgement(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow,
			input,
			attempt,
			previousState,
			"pending_economics_conflict",
			err.Error(),
			&reason,
			now,
			output,
		)
	}

	merged := mergePendingSession(attempt, input.Data)
	if merged.IdentityConflict {
		reason := model.PaymentReferenceMismatchPaymentReconciliationReason
		return uc.persistPermanentInboxFailureWithAcknowledgement(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow,
			input,
			attempt,
			previousState,
			"pending_session_identity_conflict",
			"pending reconciliation financial session identity conflicts with local payment attempt",
			&reason,
			now,
			output,
		)
	}

	if !merged.Complete {
		return uc.keepPendingIncomplete(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			attempt,
			previousState,
			"financial core pending response lacks an actionable payment session",
			now,
			output,
		)
	}

	if err := extcheckoutpayment.ValidateActionablePaymentSession(
		extcheckoutpayment.ActionablePaymentSession{
			PaymentIntentID:  merged.IntentID,
			PaymentSessionID: merged.SessionID,
			PaymentURL:       *merged.PaymentURL,
			ExpiresAt:        *merged.ExpiresAt,
		},
		now,
		0,
		0,
	); err != nil {
		return uc.keepPendingIncomplete(
			ctx,
			inboxQ,
			outboxQ,
			paymentAttemptQ,
			inboxID,
			input,
			attempt,
			previousState,
			err.Error(),
			now,
			output,
		)
	}

	if requestedExpiresAt, ok := uc.requestedExpiresAtForWarning(ctx, contractQ, attempt); ok {
		extcheckoutpayment.WarnIfExpiresAtLaterThanRequested(
			uc.logger,
			requestedExpiresAt,
			*merged.ExpiresAt,
			"payment_attempt_id", attempt.ID.String(),
			"source_id", attempt.ID.String(),
			"payment_operation_type", string(attempt.Purpose),
		)
	}

	submitted, err := paymentAttemptQ.MarkSubmittedFromReconciliationRequired(
		ctx,
		attempt.ID,
		merged.IntentID,
		merged.SessionID,
		merged.PaymentURL,
		merged.ExpiresAt,
		now,
		now,
		now,
	)
	if err != nil {
		return err
	}

	if err := uc.insertResolutionAudit(
		ctx,
		outboxQ,
		input,
		submitted,
		previousState,
		auditActionResolveSubmitted,
		auditReasonResolvedSubmitted,
		true,
		now,
	); err != nil {
		return err
	}

	return uc.finishReconciliationAcknowledgement(
		ctx,
		inboxQ,
		outboxQ,
		inboxID,
		input,
		submitted,
		previousState,
		submitted.State,
		installmentpaymentreconciliation.ResolvedSubmittedAcknowledgementResult,
		submitted.FinancialPaymentIntentID,
		submitted.ReconciliationReason,
		now,
		output,
		ProcessResultResolvedSubmitted,
	)
}

func (uc *UseCase) keepPendingIncomplete(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	lastErrorMessage string,
	now time.Time,
	output *ProcessOutput,
) error {
	nextAt := uc.nextReconciliationAt(attempt.ReconciliationAttemptCount + 1)
	lastError := stringPtr(lastErrorMessage)
	retriedAttempt, err := paymentAttemptQ.ScheduleReconciliationRetry(
		ctx,
		attempt.ID,
		nextAt,
		lastError,
		now,
	)
	if err != nil {
		return err
	}

	if err := uc.insertResolutionAudit(
		ctx,
		outboxQ,
		input,
		retriedAttempt,
		previousState,
		auditActionKeepRequired,
		string(model.FinancialCoreStatusStillUnknownPaymentReconciliationReason),
		false,
		now,
	); err != nil {
		return err
	}

	return uc.finishReconciliationAcknowledgement(
		ctx,
		inboxQ,
		outboxQ,
		inboxID,
		input,
		attempt,
		previousState,
		attempt.State,
		installmentpaymentreconciliation.StillReconciliationRequiredAcknowledgementResult,
		nil,
		attempt.ReconciliationReason,
		now,
		output,
		ProcessResultStillReconciliationRequired,
	)
}

func (uc *UseCase) handleSucceeded(
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
				"succeeded_replay_conflict",
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

	if attempt.State != model.ReconciliationRequiredPaymentAttemptState &&
		attempt.State != model.SubmittedPaymentAttemptState && attempt.State != model.ManualReviewRequiredPaymentAttemptState {
		reason := model.InvalidPaymentAttemptStatePaymentReconciliationReason
		return uc.persistReconciliationRequiredWithAcknowledgement(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow,
			input,
			attempt,
			previousState,
			"authoritative_success_state_conflict",
			"authoritative succeeded result conflicts with local payment attempt state",
			reason,
			now,
			output,
		)
	}

	if stringValue(input.Data.FinancialPaymentIntentID) == "" {
		reason := model.PaymentReferenceMismatchPaymentReconciliationReason
		return uc.persistPermanentInboxFailureWithAcknowledgement(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow,
			input,
			attempt,
			previousState,
			"payment_intent_required",
			"succeeded reconciliation result requires financial payment intent id",
			&reason,
			now,
			output,
		)
	}

	if input.Data.AmountMinor != attempt.AmountMinor {
		reason := model.PaymentReferenceMismatchPaymentReconciliationReason
		return uc.persistPermanentInboxFailureWithAcknowledgement(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow,
			input,
			attempt,
			previousState,
			"amount_mismatch",
			"reconciliation succeeded amount does not match payment attempt",
			&reason,
			now,
			output,
		)
	}

	if model.Currency(input.Data.Currency) != attempt.Currency {
		reason := model.PaymentReferenceMismatchPaymentReconciliationReason
		return uc.persistPermanentInboxFailureWithAcknowledgement(
			ctx,
			inboxQ,
			outboxQ,
			inboxRow,
			input,
			attempt,
			previousState,
			"currency_mismatch",
			"reconciliation succeeded currency does not match payment attempt",
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

func (uc *UseCase) handleFailed(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	now time.Time,
	output *ProcessOutput,
) error {
	if attempt.State == model.FailedPaymentAttemptState {
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

	if attempt.State != model.ReconciliationRequiredPaymentAttemptState &&
		attempt.State != model.SubmittedPaymentAttemptState && attempt.State != model.ManualReviewRequiredPaymentAttemptState {
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

	failureCode := stringValue(input.Data.FailureCode)
	if failureCode == "" {
		failureCode = "financial_core_payment_failed"
	}

	var failedAttempt model.PaymentAttempt
	var err error
	if attempt.State == model.SubmittedPaymentAttemptState || attempt.State == model.ManualReviewRequiredPaymentAttemptState {
		failedAttempt, err = paymentAttemptQ.MarkFailed(
			ctx,
			attempt.ID,
			failureCode,
			now,
			now,
		)
	} else {
		failedAttempt, err = paymentAttemptQ.MarkFailedFromReconciliationRequired(
			ctx,
			attempt.ID,
			failureCode,
			now,
			now,
			now,
		)
	}

	if err != nil {
		return err
	}

	if err := uc.insertResolutionAudit(
		ctx,
		outboxQ,
		input,
		failedAttempt,
		previousState,
		auditActionResolveFailed,
		auditReasonResolvedFailed,
		true,
		now,
	); err != nil {
		return err
	}

	return uc.finishReconciliationAcknowledgement(
		ctx,
		inboxQ,
		outboxQ,
		inboxID,
		input,
		failedAttempt,
		previousState,
		failedAttempt.State,
		installmentpaymentreconciliation.ResolvedFailedAcknowledgementResult,
		failedAttempt.FinancialPaymentIntentID,
		failedAttempt.ReconciliationReason,
		now,
		output,
		ProcessResultResolvedFailed,
	)
}

func (uc *UseCase) handleCanceled(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	contractQ repo.ContractTxQueryer,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	now time.Time,
	output *ProcessOutput,
) error {
	if attempt.State == model.CanceledPaymentAttemptState {
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

	if attempt.State != model.ReconciliationRequiredPaymentAttemptState &&
		attempt.State != model.SubmittedPaymentAttemptState && attempt.State != model.ManualReviewRequiredPaymentAttemptState {
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

	var failureCode *string
	if value := stringValue(input.Data.FailureCode); value != "" {
		failureCode = &value
	}

	var canceledAttempt model.PaymentAttempt
	var err error
	if attempt.State == model.SubmittedPaymentAttemptState || attempt.State == model.ManualReviewRequiredPaymentAttemptState {
		canceledAttempt, err = paymentAttemptQ.MarkCanceled(
			ctx,
			attempt.ID,
			failureCode,
			now,
			now,
		)
	} else {
		canceledAttempt, err = paymentAttemptQ.MarkCanceledFromReconciliationRequired(
			ctx,
			attempt.ID,
			failureCode,
			now,
			now,
			now,
		)
	}

	if err != nil {
		return err
	}

	if err := finalizeCanceledCheckoutContract(
		ctx,
		contractQ,
		installmentQ,
		installmentChargeQ,
		canceledAttempt,
		now,
	); err != nil {
		return err
	}

	if err := uc.insertResolutionAudit(
		ctx,
		outboxQ,
		input,
		canceledAttempt,
		previousState,
		auditActionResolveCanceled,
		auditReasonResolvedCanceled,
		true,
		now,
	); err != nil {
		return err
	}

	return uc.finishReconciliationAcknowledgement(
		ctx,
		inboxQ,
		outboxQ,
		inboxID,
		input,
		canceledAttempt,
		previousState,
		canceledAttempt.State,
		installmentpaymentreconciliation.ResolvedCanceledAcknowledgementResult,
		canceledAttempt.FinancialPaymentIntentID,
		canceledAttempt.ReconciliationReason,
		now,
		output,
		ProcessResultResolvedCanceled,
	)
}

// finalizeCanceledCheckoutContract closes a payment_pending checkout only after
// Financial Core has authoritatively resolved the payment as canceled/not-found.
// Installment-repayment cancellations do not change the contract lifecycle.
func finalizeCanceledCheckoutContract(
	ctx context.Context,
	contractQ repo.ContractTxQueryer,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	attempt model.PaymentAttempt,
	now time.Time,
) error {
	if attempt.Purpose != model.CheckoutFirstInstallmentFinancialCorePaymentPurpose {
		return nil
	}

	contractRow, err := contractQ.LockByID(ctx, attempt.ContractID)
	if err != nil {
		return err
	}

	if contractRow.Status != model.PaymentPendingContractStatus {
		return nil
	}

	if _, err := contractQ.MarkCanceledFromPaymentPending(ctx, contractRow.ID, now, now); err != nil {
		return err
	}

	if _, err := installmentQ.ExpireOfferedByContractID(ctx, contractRow.ID, now); err != nil {
		return err
	}

	if _, err := installmentChargeQ.CancelOfferedByContractID(ctx, contractRow.ID, now); err != nil {
		return err
	}

	return nil
}

func (uc *UseCase) handleNotFound(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	contractQ repo.ContractTxQueryer,
	installmentQ repo.InstallmentTxQueryer,
	installmentChargeQ repo.InstallmentChargeTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	attempt model.PaymentAttempt,
	previousState model.PaymentAttemptState,
	now time.Time,
	output *ProcessOutput,
) error {
	if attempt.State == model.CanceledPaymentAttemptState {
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

	if attempt.State != model.ReconciliationRequiredPaymentAttemptState &&
		attempt.State != model.SubmittedPaymentAttemptState {
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

	// Absence at lookup time is not authoritative cancellation: a previously
	// started generic FC request can still become visible. Keep the snapshot and
	// contract gate frozen until success/failure/cancellation or explicit review.
	if attempt.State == model.SubmittedPaymentAttemptState {
		updated, e := paymentAttemptQ.MarkReconciliationRequired(ctx, attempt.ID, model.FinancialCoreStatusStillUnknownPaymentReconciliationReason, nil, nil, now)
		if e != nil {
			return e
		}
		updated.Items = attempt.Items
		attempt = updated
	}
	return uc.keepPendingIncomplete(ctx, inboxQ, outboxQ, paymentAttemptQ, inboxID, input, attempt, previousState, "financial source not found; cancellation is not proven", now, output)
}
