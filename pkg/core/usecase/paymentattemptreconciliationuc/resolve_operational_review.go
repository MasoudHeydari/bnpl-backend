package paymentattemptreconciliationuc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

// ResolveOperationalReviewInput records a documented Financial Core investigation
// result for a payment attempt in manual_review_required and continues the
// workflow safely. Operators cannot invent monetary truth: payment-status
// findings are re-confirmed via GetPaymentStatus before money state changes.
type ResolveOperationalReviewInput struct {
	PaymentAttemptID         model.PaymentAttemptID
	ResolutionIdempotencyKey string
	Finding                  model.OperationalReviewFinding
	FCReference              string
	OperatorSubjectID        string
	Notes                    string
	// AllowSafeTerminationAfterNoEffect must be true for no_financial_effect_confirmed.
	// This method marks the attempt outcome-resolved as failed; callers may then
	// invoke TerminateCheckoutSafely. It never invents payment success.
	AllowSafeTerminationAfterNoEffect bool
}

// ResolveOperationalReviewOutput reports the post-resolution attempt state.
type ResolveOperationalReviewOutput struct {
	PaymentAttempt          model.PaymentAttempt
	AlreadyResolved         bool
	Recovery                *financialcorepaymentresultuc.RecoverByGetPaymentStatusOutput
	SafeTerminationEligible bool
}

// ResolveOperationalReview applies a documented FC investigation result.
func (uc *UseCase) ResolveOperationalReview(
	ctx context.Context,
	input ResolveOperationalReviewInput,
) (ResolveOperationalReviewOutput, error) {
	const op = "paymentattemptreconciliationuc.UseCase.ResolveOperationalReview"

	if input.PaymentAttemptID.IsZero() {
		return ResolveOperationalReviewOutput{}, fmt.Errorf("%s: payment attempt id is required", op)
	}

	if !input.Finding.IsValid() {
		return ResolveOperationalReviewOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: finding is invalid", op),
		)
	}

	resolutionKey := strings.TrimSpace(input.ResolutionIdempotencyKey)
	fcReference := strings.TrimSpace(input.FCReference)
	if resolutionKey == "" || fcReference == "" {
		return ResolveOperationalReviewOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: resolution_idempotency_key and fc_reference are required", op),
		)
	}

	now := uc.clock.Now().UTC()
	resolution := model.OperationalReviewResolution{
		Finding:           input.Finding,
		ResolutionKey:     resolutionKey,
		FCReference:       fcReference,
		OperatorSubjectID: strings.TrimSpace(input.OperatorSubjectID),
		Notes:             strings.TrimSpace(input.Notes),
		ResolvedAt:        now,
	}
	if err := resolution.Validate(); err != nil {
		return ResolveOperationalReviewOutput{}, cerr.InvalidArgument(err)
	}

	var (
		output            ResolveOperationalReviewOutput
		shouldRecover     bool
		recoverGeneration int
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)
			outboxQ := uc.outbox.Tx(tx)

			attempt, err := paymentAttemptQ.LockByID(ctx, input.PaymentAttemptID)
			if err != nil {
				return err
			}

			if attempt.State != model.ManualReviewRequiredPaymentAttemptState {
				if attempt.FinancialOutcomeResolved &&
					(attempt.State == model.SucceededPaymentAttemptState ||
						attempt.State == model.FailedPaymentAttemptState ||
						attempt.State == model.CanceledPaymentAttemptState) {
					output = ResolveOperationalReviewOutput{
						PaymentAttempt:  attempt,
						AlreadyResolved: true,
					}
					return nil
				}

				return cerr.Conflict(
					fmt.Errorf("%s: payment attempt state %q is not manual_review_required", op, attempt.State),
					map[string]any{"payment_attempt_id": attempt.ID},
				)
			}

			envelope, ok, parseErr := model.ParseOperationalReviewCase(
				optionalString(attempt.LastReconciliationError),
			)
			if parseErr != nil {
				return cerr.InternalError(parseErr)
			}

			if !ok {
				envelope, err = model.NewOperationalReviewCaseEnvelope(
					model.OperationalReviewEvidence{
						OperationName:  "GetPaymentStatus",
						SourceType:     "bnpl_payment_attempt",
						SourceID:       attempt.ID.String(),
						IdempotencyKey: attempt.ID.String(),
						CorrelationID:  attempt.ID.String(),
					},
					"manual_review_without_prior_envelope",
					now,
				)
				if err != nil {
					return err
				}
			}

			if envelope.Resolution != nil &&
				envelope.Resolution.ResolutionKey == resolutionKey {
				output = ResolveOperationalReviewOutput{
					PaymentAttempt:          attempt,
					AlreadyResolved:         true,
					SafeTerminationEligible: envelope.Resolution.Finding.AllowsSafeTermination(),
				}
				return nil
			}

			if envelope.Resolution != nil {
				return cerr.IdempotencyConflict(
					fmt.Errorf("%s: operational review already resolved with a different key", op),
					map[string]any{"payment_attempt_id": attempt.ID},
				)
			}

			envelope.Resolution = &resolution

			if err := uc.insertOperationalReviewResolutionAudit(
				ctx,
				outboxQ,
				attempt,
				envelope,
				now,
			); err != nil {
				return err
			}

			switch {
			case input.Finding.RequiresGetPaymentStatusConfirmation():
				intent := ""
				if attempt.FinancialPaymentIntentID != nil {
					intent = strings.TrimSpace(*attempt.FinancialPaymentIntentID)
				}

				if intent == "" {
					return cerr.Conflict(
						fmt.Errorf("%s: payment status finding requires a stored payment_intent_id", op),
						map[string]any{"payment_attempt_id": attempt.ID},
					)
				}

				shouldRecover = true
				recoverGeneration = attempt.ReconciliationAttemptCount + 1
				if recoverGeneration <= 0 {
					recoverGeneration = 1
				}

				output.PaymentAttempt = attempt
				return nil

			case input.Finding.AllowsSafeTermination():
				if !input.AllowSafeTerminationAfterNoEffect {
					return cerr.InvalidArgument(
						fmt.Errorf("%s: AllowSafeTerminationAfterNoEffect must be true for no-effect findings", op),
					)
				}

				failed, failErr := paymentAttemptQ.MarkFailed(
					ctx,
					attempt.ID,
					"fc_documented_no_financial_effect",
					now,
					now,
				)
				if failErr != nil {
					return failErr
				}

				output = ResolveOperationalReviewOutput{
					PaymentAttempt:          failed,
					SafeTerminationEligible: true,
				}
				return nil

			case input.Finding == model.OperationalReviewFindingFinancialEffectConfirmedOpen:
				// Keep manual_review_required. Documented successful monetary
				// effect without a Refund/Reverse contract must not free credit.
				output = ResolveOperationalReviewOutput{
					PaymentAttempt:          attempt,
					SafeTerminationEligible: false,
				}
				return nil

			default:
				return cerr.InvalidArgument(fmt.Errorf("%s: unsupported finding %q", op, input.Finding))
			}
		})
	})
	if err != nil {
		return ResolveOperationalReviewOutput{}, err
	}

	if !shouldRecover {
		return output, nil
	}

	recoverOut, recoverErr := uc.paymentStatus.RecoverByGetPaymentStatus(
		ctx,
		financialcorepaymentresultuc.RecoverByGetPaymentStatusInput{
			PaymentAttemptID: input.PaymentAttemptID,
			Generation:       recoverGeneration,
		},
	)
	if recoverErr != nil {
		if errorsIsStillInFlight(recoverErr) {
			return ResolveOperationalReviewOutput{}, cerr.Conflict(
				fmt.Errorf("%s: financial core payment is still in flight; cannot resolve as terminal yet", op),
				map[string]any{"payment_attempt_id": input.PaymentAttemptID},
			)
		}

		return ResolveOperationalReviewOutput{}, fmt.Errorf("%s: confirm payment status: %w", op, recoverErr)
	}

	output.Recovery = &recoverOut

	var refreshed model.PaymentAttempt
	loadErr := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			paymentAttemptQ := uc.paymentAttempt.Tx(tx)
			var err error
			refreshed, err = paymentAttemptQ.LockByID(ctx, input.PaymentAttemptID)
			return err
		})
	})
	if loadErr != nil {
		return output, loadErr
	}

	output.PaymentAttempt = refreshed
	return output, nil
}

func (uc *UseCase) insertOperationalReviewResolutionAudit(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	attempt model.PaymentAttempt,
	envelope model.OperationalReviewCaseEnvelope,
	now time.Time,
) error {
	const op = "paymentattemptreconciliationuc.UseCase.insertOperationalReviewResolutionAudit"

	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("%s: marshal envelope: %w", op, err)
	}

	event := newPaymentAttemptReconciliationAuditEvent(
		uc.idGenerator.Generate(),
		"payment_attempt.resolve_operational_review",
		string(envelope.Resolution.Finding),
		audit.SucceededOutcome,
		attempt,
		now,
		nil,
		map[string]any{
			"operational_review":         json.RawMessage(payload),
			"resolution_idempotency_key": envelope.Resolution.ResolutionKey,
			"fc_reference":               envelope.Resolution.FCReference,
			"operator_subject_id":        envelope.Resolution.OperatorSubjectID,
			"safe_termination_eligible":  envelope.Resolution.Finding.AllowsSafeTermination(),
		},
	)

	return uc.insertAuditOutbox(ctx, outboxQ, event, now)
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func errorsIsStillInFlight(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "still in flight") ||
		strings.Contains(err.Error(), "still requires_action") ||
		strings.Contains(err.Error(), "still processing"))
}
