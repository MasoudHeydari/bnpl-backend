package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
)

func TestReturnURLPolicyForEnvironment(t *testing.T) {
	t.Parallel()

	require.Equal(
		t,
		returndestination.Development(),
		returnURLPolicyForEnvironment(model.DevelopmentRuntimeEnvironment),
	)
	require.Equal(
		t,
		returndestination.Development(),
		returnURLPolicyForEnvironment(model.TestRuntimeEnvironment),
	)
	require.Equal(
		t,
		returndestination.Strict(),
		returnURLPolicyForEnvironment(model.ProductionRuntimeEnvironment),
	)
	require.Equal(
		t,
		returndestination.Strict(),
		returnURLPolicyForEnvironment(model.StagingRuntimeEnvironment),
	)
	require.Equal(
		t,
		returndestination.Strict(),
		returnURLPolicyForEnvironment(model.RuntimeEnvironment("unknown")),
	)
}
