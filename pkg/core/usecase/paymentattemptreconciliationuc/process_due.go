package paymentattemptreconciliationuc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

// ProcessDue claims due payment attempts in short transactions and processes
// every claimed attempt independently. A malformed attempt can therefore be
// quarantined without rolling back or starving the rest of the batch.
func (uc *UseCase) ProcessDue(ctx context.Context) error {
	const op = "paymentattemptreconciliationuc.UseCase.ProcessDue"

	now := uc.clock.Now().UTC()
	attempts, err := uc.claimDueAttempts(ctx, now)
	if err != nil {
		return err
	}

	var batchErr error
	for _, attempt := range attempts {
		if err := uc.processClaimedAttempt(ctx, attempt.ID, now); err != nil {
			batchErr = errors.Join(batchErr, fmt.Errorf("%s: process payment attempt %s reconciliation: %w", op, attempt.ID, err))
		}
	}

	return batchErr
}

func (uc *UseCase) claimDueAttempts(
	ctx context.Context,
	now time.Time,
) ([]model.PaymentAttempt, error) {
	const op = "paymentattemptreconciliationuc.UseCase.claimDueAttempts"

	initialRetryAt := uc.nextReconciliationAtFrom(now, 1)
	remaining := uc.batchSize

	claim := func(
		beforeState *model.PaymentAttemptState,
		fn func(repo.PaymentAttemptTxQueryer, int) ([]model.PaymentAttempt, error),
	) ([]model.PaymentAttempt, error) {
		if remaining <= 0 {
			return nil, nil
		}

		var attempts []model.PaymentAttempt
		err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
				paymentAttemptQ := uc.paymentAttempt.Tx(tx)
				claimed, err := fn(paymentAttemptQ, remaining)
				if err != nil {
					return err
				}

				if beforeState != nil {
					outboxQ := uc.outbox.Tx(tx)
					for _, attempt := range claimed {
						event := uc.newRequireReconciliationAuditEvent(*beforeState, attempt, now)
						if err := uc.insertAuditOutbox(ctx, outboxQ, event, now); err != nil {
							return err
						}
					}
				}

				attempts = claimed
				return nil
			})
		})
		if err == nil {
			remaining -= len(attempts)
		}

		return attempts, err
	}

	dueReconciliation, err := claim(nil, func(q repo.PaymentAttemptTxQueryer, limit int) ([]model.PaymentAttempt, error) {
		return q.ClaimDueReconciliationRequired(
			ctx,
			now,
			uc.nextReconciliationAtFrom(now, 1),
			now,
			limit,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("%s: claim due reconciliation-required payment attempts: %w", op, err)
	}

	submittedState := model.SubmittedPaymentAttemptState
	expiredSubmitted, err := claim(&submittedState, func(q repo.PaymentAttemptTxQueryer, limit int) ([]model.PaymentAttempt, error) {
		return q.ClaimExpiredSubmittedForReconciliation(
			ctx,
			now,
			uc.submittedExpiryGrace,
			model.PaymentResultTimeoutPaymentReconciliationReason,
			now,
			initialRetryAt,
			now,
			limit,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("%s: claim expired submitted payment attempts: %w", op, err)
	}

	// Stale created installment-repayment attempts are no longer moved into
	// reconciliation_required for automatic session rediscovery. Customer-driven
	// CreateCheckoutPayment retries (max 3) and the local expiration job own that
	// lifecycle. Submitted-payment outcome recovery uses GetPaymentStatus below.
	attempts := make([]model.PaymentAttempt, 0, len(expiredSubmitted)+len(dueReconciliation))
	attempts = append(attempts, dueReconciliation...)
	attempts = append(attempts, expiredSubmitted...)
	return attempts, nil
}

func (uc *UseCase) processClaimedAttempt(
	ctx context.Context,
	attemptID model.PaymentAttemptID,
	now time.Time,
) error {
	const op = "paymentattemptreconciliationuc.UseCase.processClaimedAttempt"

	var (
		observed           func()
		shouldRecover      bool
		recoveryGeneration int
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		observed = nil
		shouldRecover = false
		recoveryGeneration = 0

		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)
			attempt, err := paymentAttemptQ.LockByID(ctx, attemptID)
			if err != nil {
				return err
			}

			if attempt.State != model.ReconciliationRequiredPaymentAttemptState {
				return nil
			}

			// Residual session-creation uncertainty past TTL must not emit
			// automatic FC rediscovery. Exhausted budgets finalize as local
			// cancel; earlier abandoned creation expires.
			if attempt.EligibleForSessionCreationExpiration(
				now,
				model.FinancialSessionCreateExpirationSafetyInterval,
			) {
				if attempt.FinancialSessionCreateAttemptCount >= model.MaxFinancialSessionCreateAttempts {
					if err := attempt.State.ValidateTransitionTo(model.CanceledPaymentAttemptState); err != nil {
						return nil
					}

					canceled, cancelErr := paymentAttemptQ.CancelSessionCreationRetryExhausted(
						ctx,
						attempt.ID,
						attempt.State,
						now,
						now,
					)
					if cancelErr != nil {
						return cancelErr
					}

					event := uc.newSessionCreationRetryExhaustedCancelAuditEvent(attempt, canceled, now)
					observed = func() {
						uc.observer.AttemptClosedLocally(ctx, attempt.ID, attemptOutcomeCanceledRetryExhausted)
					}

					return uc.insertAuditOutbox(ctx, outboxQ, event, now)
				}

				if err := attempt.State.ValidateTransitionTo(model.ExpiredPaymentAttemptState); err != nil {
					return nil
				}

				expired, markErr := paymentAttemptQ.MarkExpired(
					ctx,
					attempt.ID,
					attempt.State,
					now,
					now,
				)
				if markErr != nil {
					return markErr
				}

				event := uc.newLocalExpirationAuditEvent(attempt, expired, now)
				observed = func() {
					uc.observer.AttemptClosedLocally(ctx, attempt.ID, attemptOutcomeExpired)
				}

				return uc.insertAuditOutbox(ctx, outboxQ, event, now)
			}

			// After the three customer-triggered CreateCheckoutPayment attempts
			// are exhausted, stop automatic session-creation rediscovery. Keep
			// financial_outcome_resolved=false. Submitted-payment outcome
			// recovery (payment_result_timeout with intent) is unaffected.
			if attempt.SuppressesAutomaticSessionCreationReconciliation() {
				if attempt.NextReconciliationAt == nil {
					return nil
				}

				_, clearErr := paymentAttemptQ.ClearNextReconciliationAt(ctx, attempt.ID, now)
				return clearErr
			}

			if attempt.ReconciliationAttemptCount >= uc.maxAttempts {
				return uc.escalateToManualReview(
					ctx,
					paymentAttemptQ,
					outboxQ,
					attempt,
					manualReviewRetryBudgetExhausted,
					manualReviewReasonRetryBudgetExhausted,
					now,
					&observed,
				)
			}

			paymentIntentID := ""
			if attempt.FinancialPaymentIntentID != nil {
				paymentIntentID = strings.TrimSpace(*attempt.FinancialPaymentIntentID)
			}

			if paymentIntentID == "" {
				// Without payment_intent_id, GetPaymentStatus cannot run. FC has
				// no consumer for the old Kafka reconciliation request topic, so
				// escalate for operational review instead of inventing success
				// or emitting a dead event.
				return uc.escalateMissingPaymentIntent(
					ctx,
					paymentAttemptQ,
					outboxQ,
					attempt,
					now,
					&observed,
				)
			}

			generation := attempt.ReconciliationAttemptCount + 1
			nextRetryAt := uc.nextReconciliationAtFrom(now, generation)
			note := "get_payment_status_recovery_scheduled"
			updated, recordErr := paymentAttemptQ.RecordReconciliationRequest(
				ctx,
				attempt.ID,
				attempt.ReconciliationAttemptCount,
				now,
				nextRetryAt,
				&note,
				now,
			)
			if recordErr != nil {
				return recordErr
			}

			event := uc.newRequestReconciliationAuditEvent(attempt, updated, paymentStatusRecoveryRequestID(attempt.ID, generation), now)
			if auditErr := uc.insertAuditOutbox(ctx, outboxQ, event, now); auditErr != nil {
				return auditErr
			}

			shouldRecover = true
			recoveryGeneration = generation
			observed = func() {
				uc.observer.ReconciliationRequested(ctx, attempt.ID, generation)
			}

			return nil
		})
	})
	if err != nil {
		return err
	}

	if observed != nil {
		observed()
	}

	if !shouldRecover {
		return nil
	}

	recoverOut, recoverErr := uc.paymentStatus.RecoverByGetPaymentStatus(
		ctx,
		financialcorepaymentresultuc.RecoverByGetPaymentStatusInput{
			PaymentAttemptID: attemptID,
			Generation:       recoveryGeneration,
		},
	)
	if recoverErr == nil {
		_ = recoverOut
		return nil
	}

	if errors.Is(recoverErr, financialcorepaymentresultuc.ErrPaymentStillInFlight) {
		// PROCESSING / REQUIRES_ACTION are not failures. The next due claim
		// retries GetPaymentStatus using the scheduled next_reconciliation_at.
		return nil
	}

	// Keep the attempt in reconciliation_required with the already-advanced
	// retry schedule. Persistent GetPaymentStatus failures escalate when the
	// retry budget is exhausted on a later claim.
	return fmt.Errorf("%s: get payment status recovery: %w", op, recoverErr)
}

func (uc *UseCase) escalateMissingPaymentIntent(
	ctx context.Context,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	attempt model.PaymentAttempt,
	now time.Time,
	observed *func(),
) error {
	evidence, err := uc.buildMissingIntentEvidence(ctx, attempt)
	if err != nil {
		return err
	}

	envelope, err := model.NewOperationalReviewCaseEnvelope(
		evidence,
		"payment_intent_id_missing_for_get_payment_status",
		now,
	)
	if err != nil {
		return err
	}

	encoded, err := model.MarshalOperationalReviewCase(envelope)
	if err != nil {
		return err
	}

	return uc.escalateToManualReview(
		ctx,
		paymentAttemptQ,
		outboxQ,
		attempt,
		encoded,
		manualReviewReasonMissingPaymentIntent,
		now,
		observed,
	)
}

func (uc *UseCase) buildMissingIntentEvidence(
	_ context.Context,
	attempt model.PaymentAttempt,
) (model.OperationalReviewEvidence, error) {
	sourceType := "bnpl_payment_attempt"
	sourceID := attempt.ID.String()
	operationName := "CreateCheckoutPayment"
	idempotencyKey := strings.TrimSpace(attempt.IdempotencyKey)

	if attempt.Purpose == model.CheckoutFirstInstallmentFinancialCorePaymentPurpose {
		sourceType = "bnpl_contract"
		sourceID = attempt.ContractID.String()
	}

	if idempotencyKey == "" {
		idempotencyKey = attempt.ID.String()
	}

	version := int64(1)
	return model.OperationalReviewEvidence{
		OperationName:  operationName,
		SourceType:     sourceType,
		SourceID:       sourceID,
		SourceVersion:  &version,
		IdempotencyKey: idempotencyKey,
		CorrelationID:  attempt.ID.String(),
	}, nil
}

func (uc *UseCase) escalateToManualReview(
	ctx context.Context,
	paymentAttemptQ repo.PaymentAttemptTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	attempt model.PaymentAttempt,
	lastError string,
	reasonCode string,
	now time.Time,
	observed *func(),
) error {
	updated, markErr := paymentAttemptQ.MarkManualReviewRequired(
		ctx,
		attempt.ID,
		lastError,
		now,
		now,
	)
	if markErr != nil {
		return markErr
	}

	event := uc.newManualReviewAuditEvent(attempt, updated, reasonCode, now)
	if auditErr := uc.insertAuditOutbox(ctx, outboxQ, event, now); auditErr != nil {
		return auditErr
	}

	*observed = func() {
		uc.observer.ManualReviewRequired(ctx, attempt.ID, reasonCode)
	}

	return nil
}

func paymentStatusRecoveryRequestID(attemptID model.PaymentAttemptID, generation int) string {
	return fmt.Sprintf("bnpl.payment_status_recovery:%s:%d", attemptID, generation)
}

func (uc *UseCase) nextReconciliationAtFrom(base time.Time, attemptCount int) time.Time {
	delay, err := installmentpaymentreconciliation.RetryDelay(uc.retryBaseDelay, uc.retryMaxDelay, attemptCount)
	if err != nil {
		delay = uc.retryMaxDelay
	}

	return uc.clock.Add(base, delay)
}
