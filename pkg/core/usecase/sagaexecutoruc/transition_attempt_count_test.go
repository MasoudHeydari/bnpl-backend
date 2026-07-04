package sagaexecutoruc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestStepAttemptCountForUpdate(t *testing.T) {
	t.Parallel()

	current := model.SagaInstance{
		Phase:            model.CompensatingSagaPhase,
		CurrentStep:      "checkout_confirm_claimed",
		StepAttemptCount: 2,
	}

	require.Equal(t, 2, stepAttemptCountForUpdate(current, model.CompensatingSagaPhase, current.CurrentStep))
	require.Equal(t, 0, stepAttemptCountForUpdate(current, model.ForwardSagaPhase, current.CurrentStep))
	require.Equal(t, 0, stepAttemptCountForUpdate(current, model.CompensatingSagaPhase, "checkout_contract_prepared"))
}
