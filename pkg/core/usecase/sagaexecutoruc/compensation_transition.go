package sagaexecutoruc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

func (uc *UseCase) applyDirectCompensationResultTx(
	ctx context.Context,
	tx repo.Tx,
	definition saga.Definition,
	instance model.SagaInstance,
	completion model.SagaForwardStepCompletion,
	result saga.StepResult,
) error {
	const op = "sagaexecutoruc.UseCase.applyDirectCompensationResultTx"

	if result.Outcome == saga.StepOutcomeCompleted {
		if result.NextStep != nil {
			return fmt.Errorf("%s: %w: direct compensation must not return next_step", op, saga.ErrInvalidStepResult)
		}

		now := uc.clock.Now().UTC()
		sagasQ := uc.sagas.Tx(tx)
		if _, err := sagasQ.ResolveForwardStepCompensation(
			ctx,
			completion.ID,
			model.SagaStepCompensated,
			now,
		); err != nil {
			return err
		}

		return uc.startOrAdvanceCompensationTx(
			ctx,
			tx,
			definition,
			instance,
			now,
		)
	}

	return uc.applyNonCompletedCompensationResultTx(ctx, tx, instance, result)
}

func (uc *UseCase) applyWorkflowCompensationResultTx(
	ctx context.Context,
	tx repo.Tx,
	definition saga.Definition,
	instance model.SagaInstance,
	completion model.SagaForwardStepCompletion,
	policy saga.Compensation,
	result saga.StepResult,
) error {
	const op = "sagaexecutoruc.UseCase.applyWorkflowCompensationResultTx"

	if result.Outcome != saga.StepOutcomeCompleted {
		return uc.applyNonCompletedCompensationResultTx(ctx, tx, instance, result)
	}

	now := uc.clock.Now().UTC()
	if result.NextStep != nil {
		if !policy.ContainsWorkflowStep(*result.NextStep) {
			return fmt.Errorf(
				"%s: %w: next step %q is outside the active compensation workflow", op, saga.ErrInvalidCompensation,
				*result.NextStep,
			)
		}

		sagasQ := uc.sagas.Tx(tx)
		_, err := sagasQ.Update(
			ctx,
			instance.ID,
			instance.Version,
			model.CompensatingSagaPhase,
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

	if !policy.IsWorkflowTerminalStep(instance.CurrentStep) {
		return fmt.Errorf(
			"%s: %w: non-terminal compensation step %q completed without next step", op, saga.ErrInvalidStepResult,
			instance.CurrentStep,
		)
	}

	sagasQ := uc.sagas.Tx(tx)
	if _, err := sagasQ.ResolveForwardStepCompensation(
		ctx, completion.ID, model.SagaStepCompensated, now,
	); err != nil {
		return err
	}

	return uc.startOrAdvanceCompensationTx(
		ctx,
		tx,
		definition,
		instance,
		now,
	)
}

func (uc *UseCase) applyNonCompletedCompensationResultTx(
	ctx context.Context,
	tx repo.Tx,
	instance model.SagaInstance,
	result saga.StepResult,
) error {
	const op = "sagaexecutoruc.UseCase.applyNonCompletedCompensationResultTx"

	q := uc.sagas.Tx(tx)
	now := uc.clock.Now().UTC()

	switch result.Outcome {
	case saga.StepOutcomeWaiting:
		_, err := q.Update(
			ctx,
			instance.ID,
			instance.Version,
			model.CompensatingSagaPhase,
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
			model.CompensatingSagaPhase,
			model.RunningSagaStatus,
			instance.CurrentStep,
			instance.StepAttemptCount,
			result.RetryAt,
			result.Reason,
			nil,
			now,
		)
		return err

	case saga.StepOutcomeReconcile:
		_, err := q.Update(
			ctx,
			instance.ID,
			instance.Version,
			model.CompensatingSagaPhase,
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
			model.CompensatingSagaPhase,
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
		_, err := q.Update(ctx,
			instance.ID,
			instance.Version,
			model.CompensatingSagaPhase,
			model.FailedSagaStatus,
			instance.CurrentStep,
			instance.StepAttemptCount,
			nil,
			result.Reason,
			&now,
			now,
		)
		return err

	case saga.StepOutcomeCompensate:
		return fmt.Errorf("%s: %w: recursive compensation is not allowed", op, saga.ErrInvalidStepResult)
	}

	return fmt.Errorf("%s: %w: unsupported compensation outcome %q", op, saga.ErrInvalidStepResult, result.Outcome)
}
