package trace

import (
	"testing"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"
)

func TestProvider_IDFormats(t *testing.T) {
	t.Parallel()

	provider := NewULIDProvider()

	_, err := ulid.Parse(provider.NewTraceID())
	require.NoError(t, err)

	_, err = ulid.Parse(provider.NewRequestID())
	require.NoError(t, err)

	_, err = ulid.Parse(provider.Generate())
	require.NoError(t, err)

	_, err = uuid.Parse(provider.NewCorrelationID())
	require.NoError(t, err)
}
