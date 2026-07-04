package paymentresult

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

func TestClassifyHandlerError_InvalidArgumentIsNonRetryable(t *testing.T) {
	t.Parallel()

	err := classifyHandlerError(cerr.InvalidArgument(errors.New("contract id is required")))

	require.Error(t, err)
	assert.True(t, messaging.IsNonRetryable(err))
}

func TestClassifyHandlerError_ConflictRemainsRetryable(t *testing.T) {
	t.Parallel()

	err := classifyHandlerError(cerr.Conflict(errors.New("payment occurred after contract payment deadline")))

	require.Error(t, err)
	assert.False(t, messaging.IsNonRetryable(err))
}

func TestClassifyHandlerError_TransientErrorRemainsRetryable(t *testing.T) {
	t.Parallel()

	transientErr := errors.New("database unavailable")
	err := classifyHandlerError(transientErr)

	require.ErrorIs(t, err, transientErr)
	assert.False(t, messaging.IsNonRetryable(err))
}
