package sagaexecutoruc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPersistStepContextDetachesCallerCancellation(t *testing.T) {
	t.Parallel()

	parent, cancel := context.WithCancel(context.Background())
	cancel()

	persisted := persistStepContext(parent)
	require.NoError(t, persisted.Err())
	require.Nil(t, persistStepContext(nil).Err())
}
