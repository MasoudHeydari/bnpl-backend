package sagaexecutoruc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

func (uc *UseCase) executeForward(
	ctx context.Context,
	definition saga.Definition,
	instance model.SagaInstance,
) error {
	const op = "sagaexecutoruc.UseCase.executeForward"

	step, err := saga.ResolveStep(definition, instance.CurrentStep)
	if err != nil {
		return err
	}

	txStep, isTx := step.(saga.TransactionalStep)
	externalStep, isExternal := step.(saga.ExternalStep)
	if isTx == isExternal {
		return fmt.Errorf(
			"%s: %w: step %q must implement exactly one of TransactionalStep or ExternalStep", op, ErrInvalidStepContract,
			step.Name(),
		)
	}

	if isTx {
		return uc.executeForwardTransactional(ctx, definition, instance, txStep)
	}

	result, err := externalStep.Execute(ctx, instance)
	if err != nil {
		return fmt.Errorf("%s: execute external saga step %q: %w", op, step.Name(), err)
	}

	if err = saga.ValidateStepResult(definition, instance, result); err != nil {
		return err
	}

	persistCtx := persistStepContext(ctx)
	return uc.pool.Conn(persistCtx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			return uc.applyForwardResultTx(ctx, tx, definition, instance, result)
		})
	})
}

func (uc *UseCase) executeForwardTransactional(
	ctx context.Context,
	definition saga.Definition,
	instance model.SagaInstance,
	step saga.TransactionalStep,
) error {
	const op = "sagaexecutoruc.UseCase.executeForwardTransactional"

	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			if err := uc.assertClaimVersionTx(ctx, tx, instance); err != nil {
				return err
			}

			result, err := step.ExecuteTx(ctx, tx, instance)
			if err != nil {
				return fmt.Errorf("%s: execute transactional saga step %q: %w", op, step.Name(), err)
			}

			if err = saga.ValidateStepResult(definition, instance, result); err != nil {
				return err
			}

			return uc.applyForwardResultTx(ctx, tx, definition, instance, result)
		})
	})
}
