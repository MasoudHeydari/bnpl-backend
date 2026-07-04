package paymentattemptreconciliationuc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
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

	createdCutoff := now.Add(-uc.createdTimeout)
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

	createdState := model.CreatedPaymentAttemptState
	staleCreated, err := claim(&createdState, func(q repo.PaymentAttemptTxQueryer, limit int) ([]model.PaymentAttempt, error) {
		return q.ClaimStaleCreatedForReconciliation(
			ctx,
			createdCutoff,
			model.PaymentSessionCreationTimeoutPaymentReconciliationReason,
			now,
			initialRetryAt,
			now,
			limit,
		)
	})
	if err != nil {
		return nil, fmt.Errorf("%s: claim stale created payment attempts: %w", op, err)
	}

	attempts := make([]model.PaymentAttempt, 0, len(staleCreated)+len(expiredSubmitted)+len(dueReconciliation))
	attempts = append(attempts, dueReconciliation...)
	attempts = append(attempts, expiredSubmitted...)
	attempts = append(attempts, staleCreated...)
	return attempts, nil
}

func (uc *UseCase) processClaimedAttempt(
	ctx context.Context,
	attemptID model.PaymentAttemptID,
	now time.Time,
) error {
	const op = "paymentattemptreconciliationuc.UseCase.processClaimedAttempt"

	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)
			contractQ := uc.contract.Tx(tx)
			attempt, err := paymentAttemptQ.LockByID(ctx, attemptID)
			if err != nil {
				return err
			}

			if attempt.State != model.ReconciliationRequiredPaymentAttemptState {
				return nil
			}

			if attempt.ReconciliationAttemptCount >= uc.maxAttempts {
				updated, markErr := paymentAttemptQ.MarkManualReviewRequired(
					ctx,
					attempt.ID,
					fmt.Sprintf("automatic reconciliation retry budget exhausted after %d attempts", attempt.ReconciliationAttemptCount),
					now,
					now,
				)
				if markErr != nil {
					return markErr
				}

				event := uc.newManualReviewAuditEvent(attempt, updated, manualReviewRetryBudgetExhausted, now)
				return uc.insertAuditOutbox(ctx, outboxQ, event, now)
			}

			generation := attempt.ReconciliationAttemptCount + 1
			nextRetryAt := uc.nextReconciliationAtFrom(now, generation)

			requestEventID := installmentpaymentreconciliation.RequestEventID(attempt.ID, generation)
			raw, buildErr := uc.buildReconciliationRequest(
				ctx,
				contractQ,
				attempt,
				generation,
				now,
			)
			if buildErr != nil {
				updated, markErr := paymentAttemptQ.MarkManualReviewRequired(
					ctx,
					attempt.ID,
					buildErr.Error(),
					now,
					now,
				)
				if markErr != nil {
					return errors.Join(buildErr, markErr)
				}

				event := uc.newManualReviewAuditEvent(attempt, updated, manualReviewRequestBuildFailed, now)
				if auditErr := uc.insertAuditOutbox(ctx, outboxQ, event, now); auditErr != nil {
					return errors.Join(buildErr, auditErr)
				}

				return nil
			}

			aggregateType := "payment_attempt"
			_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
				Destination:   uc.reconciliationRequestsDestination,
				MessageKey:    attempt.ID.String(),
				EventType:     installmentpaymentreconciliation.ReconciliationRequestedEventType,
				AggregateType: &aggregateType,
				Payload:       raw,
				CreatedAt:     now,
			})
			if err != nil {
				return fmt.Errorf("%s: insert reconciliation request outbox message: %w", op, err)
			}

			updated, recordErr := paymentAttemptQ.RecordReconciliationRequest(
				ctx,
				attempt.ID,
				attempt.ReconciliationAttemptCount,
				now,
				nextRetryAt,
				nil,
				now,
			)
			if recordErr != nil {
				return recordErr
			}

			event := uc.newRequestReconciliationAuditEvent(attempt, updated, requestEventID, now)
			return uc.insertAuditOutbox(ctx, outboxQ, event, now)
		})
	})
}

func (uc *UseCase) buildReconciliationRequest(
	ctx context.Context,
	contractQ repo.ContractTxQueryer,
	attempt model.PaymentAttempt,
	generation int,
	now time.Time,
) ([]byte, error) {
	const op = "paymentattemptreconciliationuc.UseCase.buildReconciliationRequest"

	contractRow, err := contractQ.GetByID(ctx, attempt.ContractID)
	if err != nil {
		return nil, fmt.Errorf("%s: load contract for reconciliation request: %w", op, err)
	}

	if generation <= 0 {
		generation = 1
	}

	reason := model.PaymentSessionCreationTimeoutPaymentReconciliationReason
	if attempt.ReconciliationReason != nil {
		reason = *attempt.ReconciliationReason
	}

	event := installmentpaymentreconciliation.NewRequestEvent(
		installmentpaymentreconciliation.RequestEventID(attempt.ID, generation),
		now,
		installmentpaymentreconciliation.RequestEventData{
			BNPLPaymentAttemptID:      attempt.ID,
			ContractID:                attempt.ContractID,
			InstallmentID:             attempt.InstallmentID,
			CustomerID:                contractRow.CustomerID,
			FinancialPaymentIntentID:  attempt.FinancialPaymentIntentID,
			FinancialPaymentSessionID: attempt.FinancialPaymentSessionID,
			AmountMinor:               attempt.AmountMinor,
			Currency:                  attempt.Currency,
			CurrentLocalState:         attempt.State,
			ReconciliationReason:      reason,
			ReconciliationGeneration:  generation,
			RequestedAt:               now,
		},
	)

	if err := installmentpaymentreconciliation.ValidateRequestEvent(event); err != nil {
		return nil, fmt.Errorf("%s: validate reconciliation request event: %w", op, err)
	}

	raw, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal reconciliation request payload: %w", op, err)
	}

	return raw, nil
}

func (uc *UseCase) nextReconciliationAtFrom(base time.Time, attemptCount int) time.Time {
	delay, err := installmentpaymentreconciliation.RetryDelay(uc.retryBaseDelay, uc.retryMaxDelay, attemptCount)
	if err != nil {
		delay = uc.retryMaxDelay
	}

	return uc.clock.Add(base, delay)
}
