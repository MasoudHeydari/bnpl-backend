package checkoutsessionuc

import (
	"context"
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ResolveConfirmationSagaOperationalReviewInput resumes or holds a checkout
// confirmation Saga that entered manual_review_required after a documented FC
// investigation. Operators cannot invent monetary truth: payment outcomes must
// still be confirmed via GetPaymentStatus / payment-result application before
// credit is released.
type ResolveConfirmationSagaOperationalReviewInput struct {
	SagaID                   model.SagaID
	ResolutionIdempotencyKey string
	Finding                  model.OperationalReviewFinding
	FCReference              string
	OperatorSubjectID        string
	Notes                    string
}

// ResolveConfirmationSagaOperationalReviewOutput reports saga state after resolution.
type ResolveConfirmationSagaOperationalReviewOutput struct {
	Saga            model.SagaInstance
	AlreadyResolved bool
}

// ResolveConfirmationSagaOperationalReview applies a documented FC finding to a
// confirmation Saga stuck in manual_review_required.
//
// Supported findings:
//   - no_financial_effect_confirmed → resume Saga to Running so compensation /
//     safe local cleanup can proceed (does not invent payment success)
//   - financial_effect_confirmed_open → keep manual_review_required (blocks
//     unsafe credit release without Refund/Reverse)
//   - payment_status_* → resume Saga to Running after caller has applied
//     GetPaymentStatus via payment-attempt resolution
func (uc *UseCase) ResolveConfirmationSagaOperationalReview(
	ctx context.Context,
	input ResolveConfirmationSagaOperationalReviewInput,
) (ResolveConfirmationSagaOperationalReviewOutput, error) {
	const op = "checkoutsessionuc.UseCase.ResolveConfirmationSagaOperationalReview"

	if input.SagaID.IsZero() {
		return ResolveConfirmationSagaOperationalReviewOutput{}, fmt.Errorf("%s: saga id is required", op)
	}

	if !input.Finding.IsValid() {
		return ResolveConfirmationSagaOperationalReviewOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: finding is invalid", op),
		)
	}

	resolutionKey := strings.TrimSpace(input.ResolutionIdempotencyKey)
	fcReference := strings.TrimSpace(input.FCReference)
	if resolutionKey == "" || fcReference == "" {
		return ResolveConfirmationSagaOperationalReviewOutput{}, cerr.InvalidArgument(
			fmt.Errorf("%s: resolution_idempotency_key and fc_reference are required", op),
		)
	}

	now := uc.clock.Now().UTC()
	var output ResolveConfirmationSagaOperationalReviewOutput

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sagaQ := uc.confirmationSagaRepo.Tx(tx)
			instance, err := sagaQ.LockByID(ctx, input.SagaID)
			if err != nil {
				return err
			}

			if instance.Type != checkoutConfirmationSagaType {
				return cerr.Conflict(
					fmt.Errorf("%s: saga type %q is not checkout confirmation", op, instance.Type),
				)
			}

			if instance.Status != model.ManualReviewRequiredSagaStatus {
				if instance.Status.IsTerminal() {
					output = ResolveConfirmationSagaOperationalReviewOutput{
						Saga:            instance,
						AlreadyResolved: true,
					}
					return nil
				}

				return cerr.Conflict(
					fmt.Errorf("%s: saga status %q is not manual_review_required", op, instance.Status),
				)
			}

			envelope, ok, parseErr := model.ParseOperationalReviewCase(optionalSagaLastError(instance.LastError))
			if parseErr != nil {
				return cerr.InternalError(parseErr)
			}

			if !ok {
				envelope, err = model.NewOperationalReviewCaseEnvelope(
					model.OperationalReviewEvidence{
						OperationName:  "checkout_confirmation_saga",
						SourceType:     "bnpl_saga",
						SourceID:       instance.ID.String(),
						IdempotencyKey: string(instance.BusinessKey),
						CorrelationID:  optionalStringPtr(instance.CorrelationID),
					},
					"saga_manual_review",
					now,
				)
				if err != nil {
					return err
				}
			}

			if envelope.Resolution != nil &&
				envelope.Resolution.ResolutionKey == resolutionKey {
				output = ResolveConfirmationSagaOperationalReviewOutput{
					Saga:            instance,
					AlreadyResolved: true,
				}
				return nil
			}

			if envelope.Resolution != nil {
				return cerr.IdempotencyConflict(
					fmt.Errorf("%s: operational review already resolved with a different key", op),
				)
			}

			envelope.Resolution = &model.OperationalReviewResolution{
				Finding:           input.Finding,
				ResolutionKey:     resolutionKey,
				FCReference:       fcReference,
				OperatorSubjectID: strings.TrimSpace(input.OperatorSubjectID),
				Notes:             strings.TrimSpace(input.Notes),
				ResolvedAt:        now,
			}

			encoded, err := model.MarshalOperationalReviewCase(envelope)
			if err != nil {
				return err
			}

			switch input.Finding {
			case model.OperationalReviewFindingFinancialEffectConfirmedOpen:
				updated, updateErr := sagaQ.Update(
					ctx,
					instance.ID,
					instance.Version,
					instance.Phase,
					model.ManualReviewRequiredSagaStatus,
					instance.CurrentStep,
					instance.StepAttemptCount,
					nil,
					&encoded,
					nil,
					now,
				)
				if updateErr != nil {
					return updateErr
				}

				output.Saga = updated
				return nil

			case model.OperationalReviewFindingNoFinancialEffectConfirmed,
				model.OperationalReviewFindingPaymentStatusSucceeded,
				model.OperationalReviewFindingPaymentStatusFailed,
				model.OperationalReviewFindingPaymentStatusCanceled,
				model.OperationalReviewFindingPaymentStatusExpired:
				nextAttempt := now
				updated, updateErr := sagaQ.Update(
					ctx,
					instance.ID,
					instance.Version,
					instance.Phase,
					model.RunningSagaStatus,
					instance.CurrentStep,
					0,
					&nextAttempt,
					&encoded,
					nil,
					now,
				)
				if updateErr != nil {
					return updateErr
				}

				output.Saga = updated
				return nil

			default:
				return cerr.InvalidArgument(fmt.Errorf("%s: unsupported finding %q", op, input.Finding))
			}
		})
	})
	if err != nil {
		return ResolveConfirmationSagaOperationalReviewOutput{}, err
	}

	return output, nil
}

func optionalSagaLastError(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func optionalStringPtr(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}
