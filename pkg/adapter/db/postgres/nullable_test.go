package postgres_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
)

func TestNullable_TypedNilBecomesUntypedNil(t *testing.T) {
	t.Parallel()

	var id *uuid.UUID

	require.Nil(t, postgres.Nullable(id))

	// Untyped nil is what drivers/loggers need; typed nil would still panic on Value().
	var asAny any = postgres.Nullable(id)
	require.True(t, asAny == nil)
}

func TestNullable_NonNilPreservesValue(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	got := postgres.Nullable(&id)
	require.Equal(t, &id, got)
}
