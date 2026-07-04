package financialcorepaymentreconciliationuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

// persistReconciliationApplicationConflict runs only after the payment-application
// transaction has rolled back. It reacquires the same inbox identity and records a
// durable reconciliation outcome plus acknowledgement in a fresh transaction.
func (uc *UseCase) persistReconciliationApplicationConflict(
	ctx context.Context,
	input ProcessInput,
	applicationErr *financialcorepaymentresultuc.ReconciliationApplicationError,
	now time.Time,
) (ProcessOutput, error) {
	var output ProcessOutput
	const op = "financialcorepaymentreconciliationuc.UseCase.persistReconciliationApplicationConflict"

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			inboxQ := uc.inbox.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)

			subjectType, subjectID := inboxSubjectForPaymentAttempt(input.Data.BNPLPaymentAttemptID)
			acquire, err := uc.acquireInboxEvent(ctx, inboxQ, input, subjectType, subjectID, now)
			if err != nil {
				return err
			}

			if acquire.Result == model.InboxAcquirePayloadMismatch {
				return uc.recordPayloadIdentityConflict(ctx, inboxQ, acquire.Row, input, now, &output)
			}

			if mapInboxAcquireResult(acquire.Result, &output) {
				return nil
			}

			attempt, err := paymentAttemptQ.LockByID(ctx, input.Data.BNPLPaymentAttemptID)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					return uc.persistPermanentInboxFailure(
						ctx,
						inboxQ,
						acquire.Row,
						"unknown_payment_attempt",
						"bnpl payment attempt was not found while recording reconciliation application conflict",
						now,
						&output,
					)
				}

				return err
			}

			previousState := attempt.State
			if attempt.State == model.SubmittedPaymentAttemptState {
				var paidAt *time.Time
				if input.Data.PaidAt != nil {
					value := input.Data.PaidAt.UTC()
					paidAt = &value
				}

				attempt, err = paymentAttemptQ.MarkReconciliationRequired(
					ctx,
					attempt.ID,
					applicationErr.Reason,
					input.Data.FinancialPaymentAttemptID,
					paidAt,
					now,
				)
				if err != nil {
					return err
				}
			}

			failureCode := string(applicationErr.Reason)
			failureMessage := applicationErr.Error()
			if _, err = inboxQ.MarkReconciliationRequired(
				ctx,
				acquire.Row.ID,
				&failureCode,
				&failureMessage,
				now,
				now,
			); err != nil {
				return err
			}

			if previousState != attempt.State {
				if err = uc.insertResolutionAudit(
					ctx,
					outboxQ,
					input,
					attempt,
					previousState,
					auditActionKeepRequired,
					auditReasonApplicationConflict,
					true,
					now,
				); err != nil {
					return err
				}
			}

			reason := applicationErr.Reason
			if err = uc.insertReconciliationAcknowledgement(
				ctx,
				outboxQ,
				input,
				attempt,
				previousState,
				attempt.State,
				installmentpaymentreconciliation.StillReconciliationRequiredAcknowledgementResult,
				attempt.FinancialPaymentIntentID,
				&reason,
				now,
			); err != nil {
				return fmt.Errorf("%s: insert durable reconciliation conflict acknowledgement: %w", op, err)
			}

			output.Result = ProcessResultStillReconciliationRequired
			return nil
		})
	})
	if err != nil {
		return ProcessOutput{}, err
	}

	return output, nil
}
