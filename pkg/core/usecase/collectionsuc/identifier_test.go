package collectionsuc

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/identifier/uuidgen"
	adaptertrace "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"
)

type fixedIdentifier string

func (id fixedIdentifier) Generate() string { return string(id) }

func TestCollectionsIdentifierValidation(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "invalid", uuid.Nil.String()} {
		uc := &UseCase{idGenerator: fixedIdentifier(value)}
		_, err := uc.nextUUID()
		require.Error(t, err)
	}

	want := uuid.New()
	uc := &UseCase{idGenerator: fixedIdentifier(want.String())}
	got, err := uc.nextUUID()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestCollectionsIdentifier_ProductionUUIDAdapter(t *testing.T) {
	t.Parallel()

	uc := &UseCase{idGenerator: uuidgen.New()}
	got, err := uc.nextUUID()
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, got)
}

func TestCollectionsIdentifier_RejectsULIDTraceProvider(t *testing.T) {
	t.Parallel()

	// Regression: DomainUUIDGenerator must not be wired to the Trace ULID
	// provider. nextUUID fails closed when given a non-UUID string.
	uc := &UseCase{idGenerator: adaptertrace.NewULIDProvider()}
	_, err := uc.nextUUID()
	require.Error(t, err)
}
