package sagaexecutoruc

import (
	"context"
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

// ExecuteByID claims and executes one runnable Saga instance.
func (uc *UseCase) ExecuteByID(ctx context.Context, id model.SagaID) error {
	const op = "sagaexecutoruc.UseCase.ExecuteByID"

	if err := uc.Validate(); err != nil {
		return err
	}

	now := uc.clock.Now().UTC()
	var instance model.SagaInstance
	if err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		sagasQ := uc.sagas.Conn(conn)
		claimed, err := sagasQ.ClaimByID(ctx, id, uc.WorkerID, now, uc.LeaseDuration)
		if err != nil {
			return err
		}

		instance = claimed
		return nil

	}); err != nil {
		return fmt.Errorf("%s: claim saga %s: %w", op, id, err)
	}

	return uc.executeClaimed(ctx, instance)
}

// ProcessDue claims and executes a batch of due Saga instances. Each Saga is
// processed independently so one failure does not roll back the rest of the
// claimed batch.
func (uc *UseCase) ProcessDue(ctx context.Context) error {
	const op = "sagaexecutoruc.UseCase.ProcessDue"

	if err := uc.Validate(); err != nil {
		return err
	}

	now := uc.clock.Now().UTC()
	var instances []model.SagaInstance
	if err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		sagasQ := uc.sagas.Conn(conn)
		claimed, err := sagasQ.ClaimDue(
			ctx,
			uc.WorkerID,
			uc.BatchSize,
			now,
			uc.LeaseDuration,
		)
		if err != nil {
			return err
		}

		instances = claimed
		return nil

	}); err != nil {
		return fmt.Errorf("%s: claim due sagas: %w", op, err)
	}

	var batchErr error
	for _, instance := range instances {
		if err := uc.executeClaimed(ctx, instance); err != nil {
			batchErr = errors.Join(batchErr, fmt.Errorf("%s: execute saga %s: %w", op, instance.ID, err))
		}
	}

	return batchErr
}

func (uc *UseCase) executeClaimed(ctx context.Context, instance model.SagaInstance) error {
	const op = "sagaexecutoruc.UseCase.executeClaimed"

	definition, ok := uc.definitions.Definition(instance.Type, instance.DefinitionVersion)
	if !ok {
		return fmt.Errorf(
			"%s: %w: type %q version %d", op, saga.ErrDefinitionNotFound,
			instance.Type,
			instance.DefinitionVersion,
		)
	}

	if err := saga.ValidateDefinition(definition); err != nil {
		return err
	}

	if instance.Phase == model.CompensatingSagaPhase {
		return uc.executeCompensation(ctx, definition, instance)
	}

	return uc.executeForward(ctx, definition, instance)
}
