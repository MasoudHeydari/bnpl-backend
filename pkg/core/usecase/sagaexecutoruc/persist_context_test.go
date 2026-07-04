package sagaexecutoruc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPersistStepContextDetachesCallerCancellation(t *testing.T) {
	t.Parallel()

	parent, cancel := context.WithCancel(context.Background())
	cancel()

	persisted, cancelPersist := persistStepContext(parent, time.Second)
	defer cancelPersist()

	require.NoError(t, persisted.Err())

	nilParent, cancelNil := persistStepContext(nil, time.Second)
	defer cancelNil()

	require.Nil(t, nilParent.Err())
}

func TestPersistStepContextHonorsPersistenceTimeout(t *testing.T) {
	t.Parallel()

	persisted, cancelPersist := persistStepContext(context.Background(), time.Millisecond)
	defer cancelPersist()

	deadline, ok := persisted.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(time.Millisecond), deadline, 50*time.Millisecond)
}
