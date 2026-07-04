package uuidgen_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/identifier/uuidgen"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
)

func TestProvider_GenerateValidDistinctUUIDs(t *testing.T) {
	t.Parallel()

	provider := uuidgen.New()
	var _ identifier.Generator = provider

	first := provider.Generate()
	second := provider.Generate()

	require.NotEmpty(t, first)
	require.NotEmpty(t, second)
	require.NotEqual(t, first, second)

	firstID, err := uuid.Parse(first)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, firstID)

	secondID, err := uuid.Parse(second)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, secondID)
}
