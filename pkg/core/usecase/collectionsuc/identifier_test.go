package collectionsuc

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fixedIdentifier string

func (id fixedIdentifier) Generate() string { return string(id) }
func TestCollectionsIdentifierValidation(t *testing.T) {
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
