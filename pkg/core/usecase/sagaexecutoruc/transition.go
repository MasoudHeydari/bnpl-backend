package sagaexecutoruc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

func (uc *UseCase) assertClaimVersionTx(
	ctx context.Context,
	tx repo.Tx,
	instance model.SagaInstance,
) error {
	const op = "sagaexecutoruc.UseCase.assertClaimVersionTx"

	sagasQ := uc.sagas.Tx(tx)
	current, err := sagasQ.LockByID(ctx, instance.ID)
	if err != nil {
		return err
	}

	if current.Version != instance.Version {
		return fmt.Errorf(
			"%s: %w: saga %s expected version %d, current version %d",
			op,
			repo.ErrSagaVersionConflict,
			instance.ID,
			instance.Version,
			current.Version,
		)
	}

	return nil
}

func (uc *UseCase) applyForwardResultTx(
	ctx context.Context,
	tx repo.Tx,
	definition saga.Definition,
	instance model.SagaInstance,
	result saga.StepResult,
) error {
	const op = "sagaexecutoruc.UseCase.applyForwardResultTx"

	q := uc.sagas.Tx(tx)
	now := uc.clock.Now().UTC()

	switch result.Outcome {
	case saga.StepOutcomeCompleted:
		if _, err := q.RecordForwardStepCompletion(
			ctx,
			instance.ID,
			instance.CurrentStep,
			instance.StepAttemptCount,
			now,
		); err != nil {
			return err
		}

		if result.NextStep != nil {
			_, err := q.Update(
				ctx,
				instance.ID,
				instance.Version,
				model.ForwardSagaPhase,
				model.RunningSagaStatus,
				*result.NextStep,
				0,
				nil,
				nil,
				nil,
				now,
			)

			return err
		}

		if !definition.IsForwardTerminalStep(instance.CurrentStep) {
			return fmt.Errorf(
				"%s: %w: non-terminal forward step %q completed without next step", op, saga.ErrInvalidStepResult,
				instance.CurrentStep,
			)
		}

		_, err := q.Update(
			ctx,
			instance.ID,
			instance.Version,
			model.ForwardSagaPhase,
			model.CompletedSagaStatus,
			instance.CurrentStep,
			instance.StepAttemptCount,
			nil,
			nil,
			&now,
			now,
		)

		return err

	case saga.StepOutcomeWaiting:
		_, err := q.Update(
			ctx,
			instance.ID,
			instance.Version,
			instance.Phase,
			model.WaitingSagaStatus,
			instance.CurrentStep,
			instance.StepAttemptCount,
			nil,
			nil,
			nil,
			now,
		)
		return err

	case saga.StepOutcomeRetry:
		_, err := q.Update(
			ctx,
			instance.ID,
			instance.Version,
			instance.Phase,
			model.RunningSagaStatus,
			instance.CurrentStep,
			instance.StepAttemptCount,
			result.RetryAt,
			result.Reason,
			nil,
			now,
		)
		return err

	case saga.StepOutcomeCompensate:
		return uc.startOrAdvanceCompensationTx(
			ctx,
			tx,
			definition,
			instance,
			now,
		)

	case saga.StepOutcomeReconcile:
		_, err := q.Update(
			ctx,
			instance.ID,
			instance.Version,
			instance.Phase,
			model.ReconciliationRequiredSagaStatus,
			instance.CurrentStep,
			instance.StepAttemptCount,
			nil,
			result.Reason,
			nil,
			now,
		)
		return err

	case saga.StepOutcomeManualReview:
		_, err := q.Update(
			ctx,
			instance.ID,
			instance.Version,
			instance.Phase,
			model.ManualReviewRequiredSagaStatus,
			instance.CurrentStep,
			instance.StepAttemptCount,
			nil,
			result.Reason,
			nil,
			now,
		)
		return err

	case saga.StepOutcomeFailed:
		_, err := q.Update(
			ctx,
			instance.ID,
			instance.Version,
			instance.Phase,
			model.FailedSagaStatus,
			instance.CurrentStep,
			instance.StepAttemptCount,
			nil,
			result.Reason,
			&now,
			now,
		)
		return err
	}

	return fmt.Errorf("%s: %w: unsupported outcome %q", op, saga.ErrInvalidStepResult, result.Outcome)
}

// stepAttemptCountForUpdate applies the Saga attempt-count invariant.
// A new step or phase starts at zero. Staying on the same operation must keep
// the count claimed for that operation; resetting it is rejected as a decrease.
func stepAttemptCountForUpdate(current model.SagaInstance, nextPhase model.SagaPhase, nextStep model.SagaStep) int {
	if current.Phase == nextPhase && current.CurrentStep == nextStep {
		return current.StepAttemptCount
	}

	return 0
}

func (uc *UseCase) startOrAdvanceCompensationTx(
	ctx context.Context,
	tx repo.Tx,
	definition saga.Definition,
	instance model.SagaInstance,
	now time.Time,
) error {
	q := uc.sagas.Tx(tx)

	for {
		completion, err := q.LockLatestUncompensatedForwardStepCompletion(ctx, instance.ID)
		if err != nil {
			if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				_, updateErr := q.Update(
					ctx,
					instance.ID,
					instance.Version,
					model.CompensatingSagaPhase,
					model.CompensatedSagaStatus,
					instance.CurrentStep,
					stepAttemptCountForUpdate(instance, model.CompensatingSagaPhase, instance.CurrentStep),
					nil,
					nil,
					&now,
					now,
				)

				return updateErr
			}

			return err
		}

		policy, err := saga.ResolveCompensation(definition, completion.Step)
		if err != nil {
			return err
		}

		switch policy.Mode {
		case saga.CompensationModeNone:
			if _, err := q.ResolveForwardStepCompensation(
				ctx,
				completion.ID,
				model.SagaStepCompensationNotRequired,
				now,
			); err != nil {
				return err
			}

			continue

		case saga.CompensationModeDirect:
			_, err = q.Update(
				ctx,
				instance.ID,
				instance.Version,
				model.CompensatingSagaPhase,
				model.RunningSagaStatus,
				completion.Step,
				stepAttemptCountForUpdate(instance, model.CompensatingSagaPhase, completion.Step),
				nil,
				nil,
				nil,
				now,
			)
			return err

		case saga.CompensationModeWorkflow:
			_, err = q.Update(
				ctx,
				instance.ID,
				instance.Version,
				model.CompensatingSagaPhase,
				model.RunningSagaStatus,
				*policy.EntryStep,
				stepAttemptCountForUpdate(instance, model.CompensatingSagaPhase, *policy.EntryStep),
				nil,
				nil,
				nil,
				now,
			)
			return err
		}
	}
}
