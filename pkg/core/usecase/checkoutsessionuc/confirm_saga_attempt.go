package checkoutsessionuc

import (
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

func confirmationAttemptFromSagaBusinessKey(instance model.SagaInstance) (model.ConfirmationAttemptID, error) {
	const op = "checkoutsessionuc.confirmationAttemptFromSagaBusinessKey"

	key := strings.TrimSpace(string(instance.BusinessKey))
	if key == "" {
		return model.ConfirmationAttemptID{}, fmt.Errorf("%s: saga business key is required", op)
	}

	attemptID, err := model.ParseConfirmationAttemptID(key)
	if err != nil {
		return model.ConfirmationAttemptID{}, fmt.Errorf("%s: invalid confirmation attempt business key: %w", op, err)
	}

	if attemptID.IsZero() {
		return model.ConfirmationAttemptID{}, fmt.Errorf("%s: saga business key must not be zero", op)
	}

	return attemptID, nil
}

func sessionMatchesConfirmationAttempt(
	session model.CheckoutSession,
	instance model.SagaInstance,
) (saga.StepResult, bool) {
	attemptID, err := confirmationAttemptFromSagaBusinessKey(instance)
	if err != nil {
		reason := boundedConfirmationSagaReason(err.Error())

		return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, false
	}

	if session.ConfirmationAttemptID == nil || session.ConfirmationAttemptID.IsZero() {
		reason := boundedConfirmationSagaReason(
			fmt.Sprintf("session %s confirming without confirmation attempt", session.ID),
		)

		return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, false
	}

	if *session.ConfirmationAttemptID != attemptID {
		reason := boundedConfirmationSagaReason(
			fmt.Sprintf(
				"session %s attempt %s does not match saga attempt %s",
				session.ID,
				session.ConfirmationAttemptID,
				attemptID,
			),
		)

		return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, false
	}

	return saga.StepResult{}, true
}

func confirmationSagaBusinessKey(session model.CheckoutSession) (model.SagaBusinessKey, error) {
	const op = "checkoutsessionuc.confirmationSagaBusinessKey"

	if session.ConfirmationAttemptID == nil || session.ConfirmationAttemptID.IsZero() {
		return "", fmt.Errorf("%s: session %s has no confirmation attempt", op, session.ID)
	}

	return model.SagaBusinessKey(session.ConfirmationAttemptID.String()), nil
}
