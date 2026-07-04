package sagaexecutoruc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

func (uc *UseCase) executeCompensation(
	ctx context.Context,
	definition saga.Definition,
	instance model.SagaInstance,
) error {
	const op = "sagaexecutoruc.UseCase.executeCompensation"

	completion, policy, err := uc.currentCompensation(ctx, definition, instance)
	if err != nil {
		return err
	}

	step, err := saga.ResolveStep(definition, instance.CurrentStep)
	if err != nil {
		return err
	}

	switch policy.Mode {
	case saga.CompensationModeDirect:
		if instance.CurrentStep != completion.Step {
			return fmt.Errorf(
				"%s: %w: direct compensation for step %q is positioned at %q",
				op,
				saga.ErrInvalidCompensation,
				completion.Step,
				instance.CurrentStep,
			)
		}

		return uc.executeDirectCompensation(
			ctx,
			definition,
			instance,
			completion,
			step,
		)

	case saga.CompensationModeWorkflow:
		if !policy.ContainsWorkflowStep(instance.CurrentStep) {
			return fmt.Errorf(
				"%s: %w: step %q is not part of compensation workflow for %q",
				op,
				saga.ErrInvalidCompensation,
				instance.CurrentStep,
				completion.Step,
			)
		}

		return uc.executeCompensationWorkflowStep(
			ctx,
			definition,
			instance,
			completion,
			policy,
			step,
		)

	case saga.CompensationModeNone:
		return fmt.Errorf(
			"%s: %w: compensation with mode none must be resolved before execution", op, saga.ErrInvalidCompensation,
		)
	}

	return fmt.Errorf("%s: %w: unsupported compensation mode %q", op, saga.ErrInvalidCompensation, policy.Mode)
}

func (uc *UseCase) currentCompensation(
	ctx context.Context,
	definition saga.Definition,
	instance model.SagaInstance,
) (model.SagaForwardStepCompletion, saga.Compensation, error) {
	var completion model.SagaForwardStepCompletion
	const op = "sagaexecutoruc.UseCase.currentCompensation"

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		sagasQ := uc.sagas.Conn(conn)
		items, err := sagasQ.ListForwardStepCompletions(ctx, instance.ID)
		if err != nil {
			return err
		}

		for i := len(items) - 1; i >= 0; i-- {
			if items[i].CompensationState == model.SagaStepNotCompensated {
				completion = items[i]
				return nil
			}
		}

		return fmt.Errorf("%s: %w: saga %s has no unresolved forward completion", op, saga.ErrInvalidCompensation, instance.ID)
	})
	if err != nil {
		return model.SagaForwardStepCompletion{}, saga.Compensation{}, err
	}

	policy, err := saga.ResolveCompensation(definition, completion.Step)
	if err != nil {
		return model.SagaForwardStepCompletion{}, saga.Compensation{}, err
	}

	return completion, policy, nil
}

func (uc *UseCase) executeDirectCompensation(
	ctx context.Context,
	definition saga.Definition,
	instance model.SagaInstance,
	completion model.SagaForwardStepCompletion,
	step saga.Step,
) error {
	const op = "sagaexecutoruc.UseCase.executeDirectCompensation"

	txStep, isTx := step.(saga.TransactionalCompensatableStep)
	externalStep, isExternal := step.(saga.ExternalCompensatableStep)
	if isTx == isExternal {
		return fmt.Errorf(
			"%s: %w: directly compensatable step %q must implement exactly one compensation execution contract", op, ErrInvalidStepContract,
			step.Name(),
		)
	}

	if isTx {
		return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
				if err := uc.assertClaimVersionTx(ctx, tx, instance); err != nil {
					return err
				}

				sagasQ := uc.sagas.Tx(tx)
				locked, err := sagasQ.LockLatestUncompensatedForwardStepCompletion(ctx, instance.ID)
				if err != nil {
					return err
				}

				if locked.ID != completion.ID {
					return fmt.Errorf("%s: %w: compensation target changed", op, repo.ErrSagaVersionConflict)
				}

				result, err := txStep.CompensateTx(ctx, tx, instance)
				if err != nil {
					return err
				}

				if err = saga.ValidateStepResult(definition, instance, result); err != nil {
					return err
				}

				return uc.applyDirectCompensationResultTx(
					ctx,
					tx,
					definition,
					instance,
					locked,
					result,
				)
			})
		})
	}

	result, err := externalStep.Compensate(ctx, instance)
	if err != nil {
		return err
	}

	if err = saga.ValidateStepResult(definition, instance, result); err != nil {
		return err
	}

	persistCtx, cancelPersist := persistStepContext(ctx, uc.PersistenceTimeout)
	defer cancelPersist()

	return uc.pool.Conn(persistCtx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sagasQ := uc.sagas.Tx(tx)
			locked, err := sagasQ.LockLatestUncompensatedForwardStepCompletion(ctx, instance.ID)
			if err != nil {
				return err
			}

			if locked.ID != completion.ID {
				return fmt.Errorf("%s: %w: compensation target changed", op, repo.ErrSagaVersionConflict)
			}

			return uc.applyDirectCompensationResultTx(
				ctx,
				tx,
				definition,
				instance,
				locked,
				result,
			)
		})
	})
}

func (uc *UseCase) executeCompensationWorkflowStep(
	ctx context.Context,
	definition saga.Definition,
	instance model.SagaInstance,
	completion model.SagaForwardStepCompletion,
	policy saga.Compensation,
	step saga.Step,
) error {
	const op = "sagaexecutoruc.UseCase.executeCompensationWorkflowStep"

	txStep, isTx := step.(saga.TransactionalStep)
	externalStep, isExternal := step.(saga.ExternalStep)
	if isTx == isExternal {
		return fmt.Errorf("%s: %w: workflow step %q must implement exactly one execution contract", op, ErrInvalidStepContract, step.Name())
	}

	if isTx {
		return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
				if err := uc.assertClaimVersionTx(ctx, tx, instance); err != nil {
					return err
				}

				sagasQ := uc.sagas.Tx(tx)
				locked, err := sagasQ.LockLatestUncompensatedForwardStepCompletion(ctx, instance.ID)
				if err != nil {
					return err
				}

				if locked.ID != completion.ID {
					return fmt.Errorf("%s: %w: compensation target changed", op, repo.ErrSagaVersionConflict)
				}

				result, err := txStep.ExecuteTx(ctx, tx, instance)
				if err != nil {
					return err
				}

				if err = saga.ValidateStepResult(definition, instance, result); err != nil {
					return err
				}

				return uc.applyWorkflowCompensationResultTx(
					ctx,
					tx,
					definition,
					instance,
					locked,
					policy,
					result,
				)
			})
		})
	}

	result, err := externalStep.Execute(ctx, instance)
	if err != nil {
		return err
	}

	if err = saga.ValidateStepResult(definition, instance, result); err != nil {
		return err
	}

	persistCtx, cancelPersist := persistStepContext(ctx, uc.PersistenceTimeout)
	defer cancelPersist()

	return uc.pool.Conn(persistCtx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sagasQ := uc.sagas.Tx(tx)
			locked, err := sagasQ.LockLatestUncompensatedForwardStepCompletion(ctx, instance.ID)
			if err != nil {
				return err
			}

			if locked.ID != completion.ID {
				return fmt.Errorf("%s: %w: compensation target changed", op, repo.ErrSagaVersionConflict)
			}

			return uc.applyWorkflowCompensationResultTx(
				ctx,
				tx,
				definition,
				instance,
				locked,
				policy,
				result,
			)
		})
	})
}
