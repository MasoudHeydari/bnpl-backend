package financialcorepaymentsessionuc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Process applies a Financial Core installment payment-session-created event durably.
func (uc *UseCase) Process(ctx context.Context, input ProcessInput) (ProcessOutput, error) {
	input = normalizeProcessInput(input)

	if err := validateMinimumTrustedEnvelope(input); err != nil {
		return ProcessOutput{}, err
	}

	now := uc.clock.Now().UTC()
	var output ProcessOutput

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			inboxQ := uc.inbox.Tx(tx)
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)

			subjectType, subjectID := inboxSubjectForPaymentAttempt(input.BNPLPaymentAttemptID)
			extRefType, extRef := inboxExternalReference(input)

			acquire, acquireErr := uc.acquireInboxEvent(
				ctx,
				inboxQ,
				input,
				subjectType,
				subjectID,
				extRefType,
				extRef,
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

			if err := validateBusinessInput(input, now); err != nil {
				return uc.persistPermanentInboxFailure(
					ctx,
					inboxQ,
					acquire.Row,
					"invalid_payment_session_payload",
					err.Error(),
					now,
					&output,
				)
			}

			attempt, lockErr := paymentAttemptQ.LockByIDWithItems(ctx, input.BNPLPaymentAttemptID)
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

			if err := validateAttemptMatchesInput(attempt, input); err != nil {
				return uc.persistPermanentInboxFailure(
					ctx,
					inboxQ,
					acquire.Row,
					"payment_attempt_mismatch",
					err.Error(),
					now,
					&output,
				)
			}

			switch attempt.State {
			case model.CreatedPaymentAttemptState:
				if usableErr := validateUsablePaymentSession(input, now); usableErr != nil {
					return uc.persistPermanentInboxFailure(
						ctx,
						inboxQ,
						acquire.Row,
						"expired_payment_session",
						usableErr.Error(),
						now,
						&output,
					)
				}

				submitted, submitErr := paymentAttemptQ.MarkSubmitted(
					ctx,
					attempt.ID,
					input.FinancialPaymentIntentID,
					input.FinancialPaymentSessionID,
					optionalPaymentURL(input.PaymentURL),
					optionalExpiresAt(input.ExpiresAt),
					input.EventOccurredAt.UTC(),
					now,
				)
				if submitErr != nil {
					return submitErr
				}

				if input.FinancialPaymentAttemptID != nil {
					if _, enrichErr := paymentAttemptQ.EnrichFinancialPaymentIdentityIfAbsent(
						ctx,
						submitted.ID,
						input.FinancialPaymentIntentID,
						optionalProviderAttemptID(&input.FinancialPaymentSessionID),
						optionalProviderAttemptID(input.FinancialPaymentAttemptID),
						now,
					); enrichErr != nil {
						return enrichErr
					}
				}

				if err := uc.insertPaymentSessionSubmittedAudit(
					ctx,
					outboxQ,
					acquire.Row,
					input,
					attempt.State,
					submitted,
					now,
				); err != nil {
					return err
				}

				if _, err := inboxQ.MarkProcessed(ctx, acquire.Row.ID, now, now); err != nil {
					return err
				}

				output.Result = ProcessResultSessionSubmitted
				return nil

			case model.ReconciliationRequiredPaymentAttemptState:
				if usableErr := validateUsablePaymentSession(input, now); usableErr != nil {
					return uc.persistPermanentInboxFailure(
						ctx,
						inboxQ,
						acquire.Row,
						"expired_payment_session",
						usableErr.Error(),
						now,
						&output,
					)
				}

				if attempt.ReconciliationReason == nil ||
					*attempt.ReconciliationReason != model.PaymentSessionCreationTimeoutPaymentReconciliationReason {
					return uc.persistSessionIdentityConflict(
						ctx,
						inboxQ,
						paymentAttemptQ,
						outboxQ,
						acquire.Row,
						input,
						attempt.ID,
						now,
						&output,
					)
				}

				submitted, submitErr := paymentAttemptQ.MarkSubmittedFromReconciliationRequired(
					ctx,
					attempt.ID,
					input.FinancialPaymentIntentID,
					input.FinancialPaymentSessionID,
					optionalPaymentURL(input.PaymentURL),
					optionalExpiresAt(input.ExpiresAt),
					input.EventOccurredAt.UTC(),
					now,
					now,
				)
				if submitErr != nil {
					return submitErr
				}

				if input.FinancialPaymentAttemptID != nil {
					if _, enrichErr := paymentAttemptQ.EnrichFinancialPaymentIdentityIfAbsent(
						ctx,
						submitted.ID,
						input.FinancialPaymentIntentID,
						optionalProviderAttemptID(&input.FinancialPaymentSessionID),
						optionalProviderAttemptID(input.FinancialPaymentAttemptID),
						now,
					); enrichErr != nil {
						return enrichErr
					}
				}

				if err := uc.insertPaymentSessionSubmittedAudit(
					ctx,
					outboxQ,
					acquire.Row,
					input,
					attempt.State,
					submitted,
					now,
				); err != nil {
					return err
				}

				if _, err := inboxQ.MarkProcessed(ctx, acquire.Row.ID, now, now); err != nil {
					return err
				}

				output.Result = ProcessResultSessionSubmitted
				return nil

			case model.SubmittedPaymentAttemptState:
				if sessionIdentityMatches(attempt, input) {
					if _, err := inboxQ.MarkProcessed(ctx, acquire.Row.ID, now, now); err != nil {
						return err
					}

					output.Result = ProcessResultEventAlreadyProcessed
					return nil
				}

				return uc.persistSessionIdentityConflict(
					ctx,
					inboxQ,
					paymentAttemptQ,
					outboxQ,
					acquire.Row,
					input,
					attempt.ID,
					now,
					&output,
				)

			case model.SucceededPaymentAttemptState,
				model.FailedPaymentAttemptState,
				model.CanceledPaymentAttemptState:
				// A terminal payment result may arrive before the session-created event. Preserve
				// the terminal state, enrich only missing Financial Core identifiers, and consume
				// this late session event idempotently.
				if _, enrichErr := paymentAttemptQ.EnrichFinancialPaymentSessionIfAbsent(
					ctx,
					attempt.ID,
					input.FinancialPaymentIntentID,
					input.FinancialPaymentSessionID,
					optionalProviderAttemptID(input.FinancialPaymentAttemptID),
					strings.TrimSpace(input.PaymentURL),
					input.ExpiresAt.UTC(),
					now,
				); enrichErr != nil {
					return uc.persistPermanentInboxFailure(
						ctx,
						inboxQ,
						acquire.Row,
						"payment_attempt_mismatch",
						enrichErr.Error(),
						now,
						&output,
					)
				}

				if _, err := inboxQ.MarkProcessed(ctx, acquire.Row.ID, now, now); err != nil {
					return err
				}

				output.Result = ProcessResultEventAlreadyProcessed
				return nil

			default:
				return uc.persistPermanentInboxFailure(
					ctx,
					inboxQ,
					acquire.Row,
					"invalid_payment_attempt_state",
					fmt.Sprintf("payment attempt state %q cannot accept payment session", attempt.State),
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

type inboxAcquire struct {
	Result model.InboxAcquireResult
	Row    model.Inbox
}

func (uc *UseCase) acquireInboxEvent(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	input ProcessInput,
	subjectType *string,
	subjectID *string,
	externalReferenceType *string,
	externalReference *string,
	now time.Time,
) (inboxAcquire, error) {
	payloadFingerprint, err := uc.paymentSessionPayloadFingerprint(input)
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
			ConsumerName:          model.FinancialCoreInstallmentPaymentSessionInboxConsumerName,
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
	const op = "financialcorepaymentsessionuc.UseCase.persistPermanentInboxFailure"

	if inboxRow.Status == model.FailedInboxStatus {
		output.Result = ProcessResultPermanentlyRejected
		return nil
	}

	if inboxRow.Status != model.ReceivedInboxStatus {
		return inboxNotReceivableConflictError(fmt.Errorf("%s: inbox row is not receivable for permanent failure", op))
	}

	if _, err := inboxQ.MarkFailed(
		ctx,
		inboxRow.ID,
		failureCode,
		failureMessage,
		now,
		now,
	); err != nil {
		return err
	}

	output.Result = ProcessResultPermanentlyRejected

	return nil
}

func (uc *UseCase) persistSessionIdentityConflict(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxRow model.Inbox,
	input ProcessInput,
	attemptID model.PaymentAttemptID,
	now time.Time,
	output *ProcessOutput,
) error {
	locked, err := paymentAttemptQ.LockByID(ctx, attemptID)
	if err != nil {
		return err
	}

	switch locked.State {
	case model.SubmittedPaymentAttemptState:
		reconciliationRequired, markErr := paymentAttemptQ.MarkReconciliationRequired(
			ctx,
			locked.ID,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			optionalProviderAttemptID(nil),
			nil,
			now,
		)
		if markErr != nil {
			return markErr
		}

		if auditErr := uc.insertPaymentSessionIdentityConflictAudit(
			ctx,
			outboxQ,
			inboxRow,
			input,
			locked.State,
			reconciliationRequired,
			now,
		); auditErr != nil {
			return auditErr
		}
	}

	if inboxRow.Status == model.FailedInboxStatus {
		output.Result = ProcessResultSessionIdentityConflict
		return nil
	}

	if _, err := inboxQ.MarkFailed(
		ctx,
		inboxRow.ID,
		"financial_core_session_identity_conflict",
		"financial core payment session identity conflicts with local attempt",
		now,
		now,
	); err != nil {
		return err
	}

	output.Result = ProcessResultSessionIdentityConflict

	return nil
}
