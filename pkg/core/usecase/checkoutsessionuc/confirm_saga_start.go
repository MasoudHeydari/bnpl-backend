package checkoutsessionuc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// startConfirmSagaTx claims open → confirming and creates the confirmation Saga
// in the same local transaction. There is never a durable confirming session
// without a resumable Saga instance for that business key.
func (uc *UseCase) startConfirmSagaTx(
	ctx context.Context,
	tx repo.Tx,
	session model.CheckoutSession,
	now time.Time,
) (model.CheckoutSession, model.SagaID, error) {
	const op = "checkoutsessionuc.UseCase.startConfirmSagaTx"

	definition := uc.confirmationSagaDefinition

	subjectType := checkoutConfirmationSagaSubjectType
	subjectID := session.ID.String()
	businessKey := model.SagaBusinessKey(session.ID.String())

	var correlationID *string
	if value := strings.TrimSpace(trace.CorrelationID(ctx)); value != "" {
		correlationID = &value
	}

	sagaQ := uc.confirmationSagaRepo.Tx(tx)
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
		nil,
		now,
		now,
	)
	if err != nil {
		return model.CheckoutSession{}, model.SagaID{}, err
	}

	sessionQ := uc.checkoutSession.Tx(tx)
	if transitionErr := session.Status.ValidateTransitionTo(model.ConfirmingCheckoutSessionStatus); transitionErr != nil {
		return model.CheckoutSession{}, model.SagaID{}, cerr.InternalError(fmt.Errorf("%s: %w", op, transitionErr))
	}

	claimed, err := sessionQ.MarkConfirming(ctx, session.ID, now)
	if err != nil {
		return model.CheckoutSession{}, model.SagaID{}, err
	}

	initialStep, err := saga.ResolveStep(definition, definition.InitialStep())
	if err != nil {
		return model.CheckoutSession{}, model.SagaID{}, err
	}

	txStep, ok := initialStep.(saga.TransactionalStep)
	if !ok {
		return model.CheckoutSession{}, model.SagaID{}, fmt.Errorf(
			"%s: confirmation initial step %q is not transactional", op, initialStep.Name(),
		)
	}

	result, err := txStep.ExecuteTx(ctx, tx, instance)
	if err != nil {
		return model.CheckoutSession{}, model.SagaID{}, err
	}

	if err := saga.ValidateStepResult(definition, instance, result); err != nil {
		return model.CheckoutSession{}, model.SagaID{}, err
	}

	if result.Outcome != saga.StepOutcomeCompleted || result.NextStep == nil {
		return model.CheckoutSession{}, model.SagaID{}, fmt.Errorf(
			"%s: confirmation initial step must complete with a next step", op,
		)
	}

	// The generic executor bumps StepAttemptCount on claim before recording
	// completion. This in-TX start path does not claim, so bump explicitly.
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
		return model.CheckoutSession{}, model.SagaID{}, err
	}

	if _, err := sagaQ.RecordForwardStepCompletion(
		ctx,
		attempted.ID,
		attempted.CurrentStep,
		attempted.StepAttemptCount,
		now,
	); err != nil {
		return model.CheckoutSession{}, model.SagaID{}, err
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
		return model.CheckoutSession{}, model.SagaID{}, err
	}

	return claimed, advanced.ID, nil
}
