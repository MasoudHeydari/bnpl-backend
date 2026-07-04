package health_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/health"
)

func TestDatabaseDependencyChecker_NilPoolFailsAtConstruction(t *testing.T) {
	t.Parallel()

	_, err := health.NewDatabaseDependencyChecker(nil, nil)
	require.Error(t, err)
	require.ErrorContains(t, err, "pool is nil")
}
