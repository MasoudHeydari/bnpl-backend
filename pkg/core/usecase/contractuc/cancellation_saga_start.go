package contractuc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

func (uc *UseCase) startContractCancellationSagaTx(
	ctx context.Context,
	tx repo.Tx,
	contractRow model.Contract,
	input CancelInput,
	fingerprint string,
	idempotencyKey string,
	financialCoreIdempotencyKey string,
	now time.Time,
) (model.Contract, model.SagaID, error) {
	const op = "contractuc.UseCase.startContractCancellationSagaTx"

	definition := uc.cancellationSagaDefinition

	subjectType := contractCancellationSagaSubjectType
	subjectID := contractRow.ID.String()
	businessKey := model.SagaBusinessKey(
		fmt.Sprintf("%s:%s", contractRow.ID, strings.TrimSpace(idempotencyKey)),
	)

	var correlationID *string
	if value := strings.TrimSpace(trace.CorrelationID(ctx)); value != "" {
		correlationID = &value
	}

	causation := strings.TrimSpace(idempotencyKey)
	var causationID *string
	if causation != "" {
		causationID = &causation
	}

	sagaQ := uc.cancellationSagaRepo.Tx(tx)
	instance, err := sagaQ.Create(
		ctx,
		definition.Type(),
		definition.Version(),
		businessKey,
		&subjectType,
		&subjectID,
		model.ForwardSagaPhase,
		model.RunningSagaStatus,
		definition.InitialStep(),
		nil,
		correlationID,
		causationID,
		now,
		now,
	)
	if err != nil {
		return model.Contract{}, model.SagaID{}, err
	}

	contractQ := uc.contract.Tx(tx)
	updated, err := contractQ.MarkCanceling(
		ctx,
		contractRow.ID,
		idempotencyKey,
		input.Reason,
		fingerprint,
		financialCoreIdempotencyKey,
		input.TenantID,
		input.ContextID,
		now,
		now,
	)
	if err != nil {
		return model.Contract{}, model.SagaID{}, err
	}

	initialStep, err := saga.ResolveStep(definition, definition.InitialStep())
	if err != nil {
		return model.Contract{}, model.SagaID{}, err
	}

	txStep, ok := initialStep.(saga.TransactionalStep)
	if !ok {
		return model.Contract{}, model.SagaID{}, fmt.Errorf(
			"%s: contract cancellation initial step %q is not transactional", op, initialStep.Name(),
		)
	}

	result, err := txStep.ExecuteTx(ctx, tx, instance)
	if err != nil {
		return model.Contract{}, model.SagaID{}, err
	}

	if err := saga.ValidateStepResult(definition, instance, result); err != nil {
		return model.Contract{}, model.SagaID{}, err
	}

	if result.Outcome != saga.StepOutcomeCompleted || result.NextStep == nil {
		return model.Contract{}, model.SagaID{}, fmt.Errorf(
			"%s: contract cancellation initial step must complete with a next step", op,
		)
	}

	// The generic executor bumps StepAttemptCount on claim before recording
	// completion. This in-TX start path does not claim, so bump explicitly to
	// satisfy RecordForwardStepCompletion's positive attempt-count contract.
	attempted, err := sagaQ.Update(
		ctx,
		instance.ID,
		instance.Version,
		model.ForwardSagaPhase,
		model.RunningSagaStatus,
		instance.CurrentStep,
		1,
		nil,
		nil,
		nil,
		now,
	)
	if err != nil {
		return model.Contract{}, model.SagaID{}, err
	}

	if _, err := sagaQ.RecordForwardStepCompletion(
		ctx,
		attempted.ID,
		attempted.CurrentStep,
		attempted.StepAttemptCount,
		now,
	); err != nil {
		return model.Contract{}, model.SagaID{}, err
	}

	advanced, err := sagaQ.Update(
		ctx,
		attempted.ID,
		attempted.Version,
		model.ForwardSagaPhase,
		model.RunningSagaStatus,
		*result.NextStep,
		0,
		nil,
		nil,
		nil,
		now,
	)
	if err != nil {
		return model.Contract{}, model.SagaID{}, err
	}

	return updated, advanced.ID, nil
}
