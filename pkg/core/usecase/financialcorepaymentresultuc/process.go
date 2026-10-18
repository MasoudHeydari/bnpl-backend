package financialcorepaymentresultuc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/paymentapplication"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

// ProcessInput carries a Financial Core payment result with inbox metadata.
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

	// PaymentPurpose classifies the Financial Core payment purpose.
	PaymentPurpose model.FinancialCorePaymentPurpose

	// ContractID identifies the contract.
	ContractID model.ContractID
	// InstallmentID identifies the installment.
	InstallmentID model.InstallmentID

	// InvoiceID identifies the Financial Core invoice.
	InvoiceID string
	// PaymentIntentID identifies the Financial Core payment intent.
	PaymentIntentID string
	// PaymentAttemptID identifies the payment attempt in Financial Core or BNPL context.
	PaymentAttemptID string
	// PaymentSessionID identifies the Financial Core payment session.
	PaymentSessionID string

	// PaymentStatus is the normalized payment status.
	PaymentStatus model.FinancialPaymentResultStatus
	// RawPaymentStatus is the provider raw payment status string.
	RawPaymentStatus string

	// PaidAmountMinor is the paid amount in minor currency units.
	PaidAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// PaidAt is when payment occurred, if applicable.
	PaidAt time.Time

	// EventOccurredAt is when the source event occurred.
	EventOccurredAt time.Time

	// SourceType classifies the payment source type.
	SourceType string
	// SourceID identifies the payment source.
	SourceID string

	// FailureCode is the provider failure code when payment failed.
	FailureCode *string
}

// Process applies a Financial Core payment result durably.
func (uc *UseCase) Process(
	ctx context.Context,
	input ProcessInput,
) (output ProcessOutput, err error) {
	input = normalizeProcessInput(input)

	if err = validateMinimumTrustedEnvelope(input); err != nil {
		return
	}

	purpose := resolvePaymentPurpose(input)

	switch purpose {
	case model.CheckoutFirstInstallmentFinancialCorePaymentPurpose:
		input, err = uc.resolveAuthoritativeCheckoutResult(ctx, input)

	case model.InstallmentRepaymentFinancialCorePaymentPurpose:
		input, err = uc.resolveAuthoritativeInstallmentResult(ctx, input)
	}

	if err != nil {
		return
	}

	switch purpose {
	case model.CheckoutFirstInstallmentFinancialCorePaymentPurpose:
		output, err = uc.processCheckoutFirstInstallment(ctx, input, purpose)

	case model.InstallmentRepaymentFinancialCorePaymentPurpose:
		output, err = uc.processInstallmentRepayment(ctx, input, purpose)

	default:
		output, err = uc.processUnsupportedPaymentPurpose(ctx, input, purpose)
	}

	if err != nil {
		return
	}

	// Record only after a durable payment-attempt completion. Inbox/idempotent
	// replays that do not newly complete an attempt are excluded by result.
	if isCompletedPaymentAttemptResult(output.Result) {
		uc.businessMetrics.PaymentAttemptCompleted(ctx, string(input.PaymentStatus), "financial_core")
	}

	return output, nil
}

func (uc *UseCase) processUnsupportedPaymentPurpose(
	ctx context.Context,
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
) (ProcessOutput, error) {
	now := uc.clock.Now().UTC()
	var output ProcessOutput

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			inboxQ := uc.inbox.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)

			acquire, acquireErr := uc.acquireInboxEvent(ctx, inboxQ, input, purpose, nil, nil, nil, nil, now)
			if acquireErr != nil {
				return acquireErr
			}

			if acquire.Result == model.InboxAcquirePayloadMismatch {
				return uc.recordPayloadIdentityConflict(ctx, inboxQ, outboxQ, acquire.Row, input, purpose, now, &output)
			}

			if mapInboxAcquireResult(acquire.Result, &output) {
				return nil
			}

			if validationErr := validatePaymentResultTemporalInput(input, now); validationErr != nil {
				return uc.persistPermanentInboxFailure(
					ctx, inboxQ, acquire.Row, "invalid_payment_result_payload", validationErr.Error(), now, &output,
				)
			}

			if input.PaymentStatus == model.SucceededFinancialPaymentResultStatus &&
				input.PaymentIntentID != "" {
				return uc.persistCheckoutPaymentReconciliation(
					ctx,
					inboxQ,
					outboxQ,
					acquire.Row.ID,
					input,
					unsupportedPurposeAck(input, purpose),
					model.UnsupportedPaymentPurposePaymentReconciliationReason,
					now,
					&output,
				)
			}

			failureMessage := fmt.Sprintf("unsupported payment purpose %q", purpose)
			if purpose == "" {
				failureMessage = "unsupported payment purpose"
			}

			return uc.persistPermanentInboxFailure(
				ctx,
				inboxQ,
				acquire.Row,
				"unsupported_payment_purpose",
				failureMessage,
				now,
				&output,
			)
		})
	})
	if err != nil {
		return ProcessOutput{}, err
	}

	return output, nil
}

func (uc *UseCase) processCheckoutFirstInstallment(
	ctx context.Context,
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
) (ProcessOutput, error) {
	now := uc.clock.Now().UTC()
	var output ProcessOutput

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			inboxQ := uc.inbox.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)

			subjectType, subjectID := resolveCheckoutInboxSubject(input)
			extRefType := string(model.FinancialPaymentIntentInboxExternalReferenceType)
			intentID := input.PaymentIntentID

			acquire, acquireErr := uc.acquireInboxEvent(
				ctx,
				inboxQ,
				input,
				purpose,
				subjectType,
				subjectID,
				&extRefType,
				&intentID,
				now,
			)
			if acquireErr != nil {
				return acquireErr
			}

			if acquire.Result == model.InboxAcquirePayloadMismatch {
				return uc.recordPayloadIdentityConflict(ctx, inboxQ, outboxQ, acquire.Row, input, purpose, now, &output)
			}

			if mapInboxAcquireResult(acquire.Result, &output) {
				return nil
			}

			if validationErr := validatePaymentResultTemporalInput(input, now); validationErr != nil {
				return uc.persistPermanentInboxFailure(
					ctx, inboxQ, acquire.Row, "invalid_payment_result_payload", validationErr.Error(), now, &output,
				)
			}

			switch input.PaymentStatus {
			case model.SucceededFinancialPaymentResultStatus:
				if input.PaymentIntentID == "" {
					return uc.persistPermanentInboxFailure(
						ctx,
						inboxQ,
						acquire.Row,
						missingPaymentIntentFailureCode,
						missingPaymentIntentFailureMessage,
						now,
						&output,
					)
				}

				return uc.processSucceededCheckoutPaymentResult(
					ctx,
					tx,
					inboxQ,
					outboxQ,
					acquire.Row,
					input,
					purpose,
					now,
					&output,
				)

			case model.FailedFinancialPaymentResultStatus:
				return uc.finishCheckoutTerminalPaymentResult(
					ctx,
					tx,
					inboxQ,
					outboxQ,
					acquire.Row.ID,
					input,
					now,
					ProcessResultFailureRecorded,
					&output,
				)

			case model.CanceledFinancialPaymentResultStatus:
				return uc.finishCheckoutTerminalPaymentResult(
					ctx,
					tx,
					inboxQ,
					outboxQ,
					acquire.Row.ID,
					input,
					now,
					ProcessResultCancellationRecorded,
					&output,
				)

			default:
				return uc.persistPermanentInboxFailure(
					ctx,
					inboxQ,
					acquire.Row,
					paymentapplication.UnsupportedPaymentResultStatusFailureCode,
					unsupportedPaymentStatusMessage(input),
					now,
					&output,
				)
			}
		})
	})
	if err != nil {
		return ProcessOutput{}, err
	}

	if input.PaymentStatus == model.SucceededFinancialPaymentResultStatus &&
		(output.Result == ProcessResultPaymentApplied ||
			output.Result == ProcessResultAlreadyApplied ||
			output.Result == ProcessResultEventAlreadyProcessed) {
		activateInput := contractuc.ActivateFromFinancialCorePaymentResultInput{
			ContractID: input.ContractID,
			SourceType: input.SourceType,
			SourceID:   input.SourceID,
		}
		contractID, resolveErr := resolveContractID(activateInput)
		if resolveErr != nil {
			return ProcessOutput{}, resolveErr
		}

		if entitlementErr := uc.contractUC.EnsureMerchantEntitlement(ctx, contractID); entitlementErr != nil {
			return ProcessOutput{}, entitlementErr
		}
	}

	return output, nil
}

func (uc *UseCase) finishCheckoutTerminalPaymentResult(
	ctx context.Context,
	tx repo.Tx,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	now time.Time,
	result ProcessResult,
	output *ProcessOutput,
) error {
	paymentAttemptQ := uc.paymentAttempt.Tx(tx)
	resolvedAttempt, err := resolvePaymentAttemptForResult(
		ctx,
		paymentAttemptQ,
		input,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	)
	if err != nil {
		return uc.persistUnmatchedTerminalCheckoutResult(
			ctx,
			inboxQ,
			outboxQ,
			inboxID,
			input,
			"invalid_payment_attempt_reference",
			err.Error(),
			nil,
			model.ContractID{},
			now,
			output,
		)
	}

	if resolvedAttempt == nil {
		return uc.persistUnmatchedTerminalCheckoutResult(
			ctx,
			inboxQ,
			outboxQ,
			inboxID,
			input,
			"unknown_payment_attempt",
			"local checkout payment attempt not found for terminal payment result",
			nil,
			input.ContractID,
			now,
			output,
		)
	}

	contractQ := uc.contract.Tx(tx)
	contractRow, err := contractQ.LockByID(ctx, resolvedAttempt.ContractID)
	if err != nil {
		if isContractNotFoundError(err) {
			return uc.persistUnmatchedTerminalCheckoutResult(
				ctx, inboxQ, outboxQ, inboxID, input, "contract_not_found",
				"local contract not found for terminal checkout payment result",
				resolvedAttempt, resolvedAttempt.ContractID, now, output,
			)
		}

		return err
	}

	if contractRow.Status != model.PaymentPendingContractStatus &&
		contractRow.Status != model.ExpiredContractStatus {
		return uc.persistUnmatchedTerminalCheckoutResult(
			ctx, inboxQ, outboxQ, inboxID, input, "invalid_contract_state",
			"terminal checkout payment result does not match contract state",
			resolvedAttempt, contractRow.ID, now, output,
		)
	}

	attempt, err := paymentAttemptQ.LockByIDWithItems(ctx, resolvedAttempt.ID)
	if err != nil {
		return err
	}

	if attempt.ContractID != contractRow.ID {
		return uc.persistUnmatchedTerminalCheckoutResult(
			ctx, inboxQ, outboxQ, inboxID, input, "payment_reference_mismatch",
			"terminal checkout payment attempt does not belong to the locked contract",
			&attempt, contractRow.ID, now, output,
		)
	}

	if err := validateResultTargetsAttempt(
		attempt,
		input,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	); err != nil {
		return uc.persistUnmatchedTerminalCheckoutResult(
			ctx,
			inboxQ,
			outboxQ,
			inboxID,
			input,
			"payment_reference_mismatch",
			"terminal checkout payment result does not match the local payment attempt",
			&attempt,
			contractRow.ID,
			now,
			output,
		)
	}

	if input.PaymentStatus == model.FailedFinancialPaymentResultStatus &&
		attempt.State == model.FailedPaymentAttemptState {
		// The payment_pending Contract stays in place so a later Confirm can
		// mint one replacement PaymentAttempt. Canceling it would force a new
		// Contract for the same checkout.
		if err := uc.checkoutSessionUC.ApplyPaymentRetryableFailure(ctx, tx, contractRow.ID, now); err != nil {
			return err
		}

		return uc.finishTerminalInbox(ctx, inboxQ, inboxID, ProcessResultEventAlreadyProcessed, now, output)
	}

	if input.PaymentStatus == model.CanceledFinancialPaymentResultStatus &&
		attempt.State == model.CanceledPaymentAttemptState {
		if err := uc.checkoutSessionUC.ApplyPaymentRetryableFailure(ctx, tx, contractRow.ID, now); err != nil {
			return err
		}

		return uc.finishTerminalInbox(ctx, inboxQ, inboxID, ProcessResultEventAlreadyProcessed, now, output)
	}

	if attempt.State == model.ManualReviewRequiredPaymentAttemptState &&
		contractRow.Status == model.ExpiredContractStatus {
		return uc.finishTerminalInbox(ctx, inboxQ, inboxID, result, now, output)
	}

	if attempt.State != model.CreatedPaymentAttemptState &&
		attempt.State != model.SubmittedPaymentAttemptState &&
		attempt.State != model.ReconciliationRequiredPaymentAttemptState {
		return uc.persistUnmatchedTerminalCheckoutResult(
			ctx, inboxQ, outboxQ, inboxID, input, "invalid_payment_attempt_state",
			"terminal checkout result cannot be applied to the local payment attempt state",
			&attempt, contractRow.ID, now, output,
		)
	}

	previousState := attempt.State
	enrichedAttempt, enrichErr := enrichAttemptIdentityFromResult(ctx, paymentAttemptQ, attempt, input, now)
	if enrichErr != nil {
		return uc.persistUnmatchedTerminalCheckoutResult(
			ctx, inboxQ, outboxQ, inboxID, input, "payment_reference_mismatch",
			"terminal checkout result conflicts with the local payment identity",
			&attempt, contractRow.ID, now, output,
		)
	}

	switch input.PaymentStatus {
	case model.CanceledFinancialPaymentResultStatus:
		cancelCode := normalizedFailureCode(input.FailureCode)
		if _, err := paymentAttemptQ.MarkCanceled(ctx, enrichedAttempt.ID, cancelCode, input.EventOccurredAt.UTC(), now); err != nil {
			return err
		}

		reasonCode := auditReasonPaymentAttemptCanceled
		if cancelCode != nil {
			reasonCode = *cancelCode
		}

		if err := uc.insertPaymentAttemptCanceledAudit(ctx, outboxQ, input, paymentAttemptTerminalAuditFacts{
			InboxID:       inboxID,
			Attempt:       enrichedAttempt,
			PreviousState: previousState,
			CurrentState:  model.CanceledPaymentAttemptState,
			ProcessResult: result,
			ReasonCode:    reasonCode,
			ContractID:    contractRow.ID,
		}, now); err != nil {
			return err
		}

	case model.FailedFinancialPaymentResultStatus:
		reasonCode := "unknown"
		if input.FailureCode != nil && strings.TrimSpace(*input.FailureCode) != "" {
			reasonCode = strings.TrimSpace(*input.FailureCode)
		}

		if _, err := paymentAttemptQ.MarkFailed(ctx, enrichedAttempt.ID, reasonCode, input.EventOccurredAt.UTC(), now); err != nil {
			return err
		}

		if err := uc.insertPaymentAttemptFailedAudit(ctx, outboxQ, input, paymentAttemptTerminalAuditFacts{
			InboxID:       inboxID,
			Attempt:       enrichedAttempt,
			PreviousState: previousState,
			CurrentState:  model.FailedPaymentAttemptState,
			ProcessResult: result,
			ReasonCode:    reasonCode,
			ContractID:    contractRow.ID,
		}, now); err != nil {
			return err
		}

	default:
		return uc.persistUnmatchedTerminalCheckoutResult(
			ctx, inboxQ, outboxQ, inboxID, input, "unsupported_payment_status",
			"terminal checkout handler received a non-terminal status",
			&enrichedAttempt, contractRow.ID, now, output,
		)
	}

	// PaymentAttempt FAILED/CANCELED is not CheckoutSession FAILED. Reopen the
	// handoff (and bump evaluation generation) so the customer may Confirm again.
	if err := uc.checkoutSessionUC.ApplyPaymentRetryableFailure(ctx, tx, contractRow.ID, now); err != nil {
		return err
	}

	if _, err := inboxQ.MarkProcessed(ctx, inboxID, now, now); err != nil {
		return err
	}

	output.Result = result
	return nil
}

func (uc *UseCase) persistUnmatchedTerminalCheckoutResult(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	failureCode string,
	failureMessage string,
	attempt *model.PaymentAttempt,
	contractID model.ContractID,
	now time.Time,
	output *ProcessOutput,
) error {
	if _, err := inboxQ.MarkFailed(ctx, inboxID, failureCode, failureMessage, now, now); err != nil {
		return err
	}

	if err := uc.insertUnmatchedTerminalAudit(ctx, outboxQ, input, unmatchedTerminalAuditFacts{
		InboxID:     inboxID,
		FailureCode: failureCode,
		Attempt:     attempt,
		ContractID:  contractID,
	}, now); err != nil {
		return err
	}

	output.Result = ProcessResultTerminalResultUnmatched
	return nil
}

func (uc *UseCase) processInstallmentRepayment(
	ctx context.Context,
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
) (ProcessOutput, error) {
	now := uc.clock.Now().UTC()
	var output ProcessOutput

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			inboxQ := uc.inbox.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)

			subjectType, subjectID := inboxSubjectForInstallmentRepayment(input)
			extRefType := string(model.FinancialPaymentIntentInboxExternalReferenceType)
			intentID := input.PaymentIntentID

			acquire, acquireErr := uc.acquireInboxEvent(
				ctx,
				inboxQ,
				input,
				purpose,
				subjectType,
				subjectID,
				&extRefType,
				&intentID,
				now,
			)
			if acquireErr != nil {
				return acquireErr
			}

			if acquire.Result == model.InboxAcquirePayloadMismatch {
				return uc.recordPayloadIdentityConflict(ctx, inboxQ, outboxQ, acquire.Row, input, purpose, now, &output)
			}

			if mapInboxAcquireResult(acquire.Result, &output) {
				return nil
			}

			if validationErr := validatePaymentResultTemporalInput(input, now); validationErr != nil {
				return uc.persistPermanentInboxFailure(
					ctx, inboxQ, acquire.Row, "invalid_payment_result_payload", validationErr.Error(), now, &output,
				)
			}

			resolvedAttempt, resolveErr := resolvePaymentAttemptForResult(
				ctx,
				paymentAttemptQ,
				input,
				purpose,
			)
			if resolveErr != nil {
				return uc.persistPermanentInboxFailure(
					ctx,
					inboxQ,
					acquire.Row,
					"invalid_payment_attempt_reference",
					resolveErr.Error(),
					now,
					&output,
				)
			}

			switch input.PaymentStatus {
			case model.SucceededFinancialPaymentResultStatus:
				if input.PaymentIntentID == "" {
					return uc.persistPermanentInboxFailure(
						ctx,
						inboxQ,
						acquire.Row,
						missingPaymentIntentFailureCode,
						missingPaymentIntentFailureMessage,
						now,
						&output,
					)
				}

				contractQ := uc.contract.Tx(tx)
				installmentQ := uc.installment.Tx(tx)
				installmentChargeQ := uc.installmentCharge.Tx(tx)
				creditAccountQ := uc.creditAccount.Tx(tx)
				return uc.processSucceededInstallmentRepayment(
					ctx,
					inboxQ,
					outboxQ,
					paymentAttemptQ,
					contractQ,
					installmentQ,
					installmentChargeQ,
					creditAccountQ,
					acquire.Row.ID,
					input,
					purpose,
					resolvedAttempt,
					now,
					&output,
				)

			case model.FailedFinancialPaymentResultStatus, model.CanceledFinancialPaymentResultStatus:
				return uc.processTerminalInstallmentRepayment(
					ctx,
					inboxQ,
					outboxQ,
					paymentAttemptQ,
					acquire.Row.ID,
					input,
					resolvedAttempt,
					now,
					&output,
				)

			default:
				return uc.persistPermanentInboxFailure(
					ctx,
					inboxQ,
					acquire.Row,
					paymentapplication.UnsupportedPaymentResultStatusFailureCode,
					unsupportedPaymentStatusMessage(input),
					now,
					&output,
				)
			}
		})
	})
	if err != nil {
		return ProcessOutput{}, err
	}

	return output, nil
}

func (uc *UseCase) processSucceededInstallmentRepayment(
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
	resolvedAttempt *model.PaymentAttempt,
	now time.Time,
	output *ProcessOutput,
) error {
	ack := installmentAckContext(input, purpose, resolvedAttempt)

	if err := validateInstallmentRepaymentBusinessInput(input); err != nil {
		return uc.persistInstallmentPaymentReconciliation(
			ctx, inboxQ, outboxQ, paymentAttemptQ, inboxID, input, ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason, now, output,
		)
	}

	if reason := resolveInstallmentRepaymentTargetReason(input); reason != "" {
		return uc.persistInstallmentPaymentReconciliation(
			ctx, inboxQ, outboxQ, paymentAttemptQ, inboxID, input, ack,
			reason, now, output,
		)
	}

	if resolvedAttempt == nil {
		return uc.persistInstallmentPaymentReconciliation(
			ctx, inboxQ, outboxQ, paymentAttemptQ, inboxID, input, ack,
			model.UnknownPaymentAttemptPaymentReconciliationReason, now, output,
		)
	}

	attempt, lockErr := paymentAttemptQ.LockByIDWithItems(ctx, resolvedAttempt.ID)
	if lockErr != nil {
		return lockErr
	}

	ack.Attempt = &attempt

	if err := validateResultTargetsAttempt(
		attempt,
		input,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
	); err != nil {
		return uc.persistInstallmentPaymentReconciliation(
			ctx, inboxQ, outboxQ, paymentAttemptQ, inboxID, input, ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason, now, output,
		)
	}

	enrichedAttempt, enrichErr := enrichAttemptIdentityFromResult(ctx, paymentAttemptQ, attempt, input, now)
	if enrichErr != nil {
		return uc.persistInstallmentPaymentReconciliation(
			ctx, inboxQ, outboxQ, paymentAttemptQ, inboxID, input, ack,
			model.PaymentReferenceMismatchPaymentReconciliationReason, now, output,
		)
	}

	attempt = enrichedAttempt

	return uc.applySucceededInstallmentPaymentResult(
		ctx,
		inboxQ,
		outboxQ,
		paymentAttemptQ,
		contractQ,
		installmentQ,
		installmentChargeQ,
		accountQ,
		inboxID,
		input,
		purpose,
		attempt,
		now,
		output,
	)
}

func (uc *UseCase) processTerminalInstallmentRepayment(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	inboxID model.InboxID,
	input ProcessInput,
	resolvedAttempt *model.PaymentAttempt,
	now time.Time,
	output *ProcessOutput,
) error {
	if resolvedAttempt == nil {
		return uc.applyTerminalInstallmentPaymentResult(
			ctx, inboxQ, outboxQ, paymentAttemptQ, inboxID, input, nil, now, output,
		)
	}

	attempt, lockErr := paymentAttemptQ.LockByIDWithItems(ctx, resolvedAttempt.ID)
	if lockErr != nil {
		return lockErr
	}

	return uc.applyTerminalInstallmentPaymentResult(
		ctx, inboxQ, outboxQ, paymentAttemptQ, inboxID, input, &attempt, now, output,
	)
}

type inboxAcquire struct {
	Result model.InboxAcquireResult
	Row    model.Inbox
}

func (uc *UseCase) acquireInboxEvent(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
	subjectType *string,
	subjectID *string,
	externalReferenceType *string,
	externalReference *string,
	now time.Time,
) (inboxAcquire, error) {
	payloadFingerprint, err := uc.paymentResultPayloadFingerprint(input, purpose)
	if err != nil {
		return inboxAcquire{}, err
	}

	row, result, err := inboxQ.AcquireForProcessing(
		ctx,
		model.InboxEvent{
			EventID:               input.EventID,
			EventType:             input.EventType,
			EventVersion:          input.EventVersion,
			SourceService:         input.SourceService,
			CorrelationID:         input.CorrelationID,
			CausationID:           input.CausationID,
			Payload:               input.RawPayload,
			PayloadFingerprint:    payloadFingerprint,
			ConsumerName:          model.FinancialCorePaymentResultInboxConsumerName,
			SubjectType:           subjectType,
			SubjectID:             subjectID,
			ExternalReferenceType: externalReferenceType,
			ExternalReference:     externalReference,
		},
		now,
	)
	if err != nil {
		return inboxAcquire{}, err
	}

	return inboxAcquire{Result: result, Row: row}, nil
}

func (uc *UseCase) persistPermanentInboxFailure(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	inboxRow model.Inbox,
	failureCode string,
	failureMessage string,
	now time.Time,
	output *ProcessOutput,
) error {
	const op = "financialcorepaymentresultuc.UseCase.persistPermanentInboxFailure"

	if inboxRow.Status == model.FailedInboxStatus {
		output.Result = ProcessResultPermanentlyRejected
		return nil
	}

	if inboxRow.Status != model.ReceivedInboxStatus {
		return inboxNotReceivableConflictError(fmt.Errorf("%s: inbox row is not receivable for permanent failure", op))
	}

	if _, err := inboxQ.MarkFailed(ctx, inboxRow.ID, failureCode, failureMessage, now, now); err != nil {
		return err
	}

	output.Result = ProcessResultPermanentlyRejected

	return nil
}

const (
	missingPaymentIntentFailureCode    = "missing_payment_intent"
	missingPaymentIntentFailureMessage = "succeeded financial core payment result is missing financial payment intent id"
)

func stringPtr(value string) *string {
	return &value
}

func isCompletedPaymentAttemptResult(result ProcessResult) bool {
	switch result {
	case ProcessResultPaymentApplied, ProcessResultFailureRecorded, ProcessResultCancellationRecorded:
		return true

	default:
		return false
	}
}
