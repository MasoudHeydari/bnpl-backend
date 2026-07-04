package customercredituc

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestInitialLimitIdempotencyKey_BoundsLongMaterialDeterministically(t *testing.T) {
	t.Parallel()

	customerID := model.CustomerID(uuid.New())
	assessment := InitialLimitAssessmentResult{
		Outcome:       model.DeniedDecisionOutcome,
		PolicyID:      strings.Repeat("p", 128),
		PolicyVersion: strings.Repeat("v", 64),
		ReasonCodes:   []string{strings.Repeat("R", 128)},
	}

	first := initialLimitIdempotencyKey(customerID, assessment, 0)
	second := initialLimitIdempotencyKey(customerID, assessment, 0)

	require.LessOrEqual(t, len(first), maxDecisionIdempotencyKeyLength)
	require.True(t, strings.HasPrefix(first, "initial-credit-limit:sha256:"))
	require.Equal(t, first, second)

	assessment.ReasonCodes[0] = strings.Repeat("X", 128)
	require.NotEqual(t, first, initialLimitIdempotencyKey(customerID, assessment, 0))
}
