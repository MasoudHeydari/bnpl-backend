package sha256_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
)

func TestHasher_HashAndEqual(t *testing.T) {
	t.Parallel()

	hasher := sha256fingerprint.New()
	raw := "canonical-input"

	value, err := hasher.Hash(raw)
	require.NoError(t, err)
	require.NoError(t, value.Validate())
	require.Equal(t, fingerprint.SHA256Algorithm, value.Algorithm)

	sum := sha256.Sum256([]byte(raw))
	require.Equal(t, hex.EncodeToString(sum[:]), value.Digest)
	require.True(t, hasher.Equal(raw, value.String()))
	require.False(t, hasher.Equal("other", value.String()))
	require.False(t, hasher.Equal(raw, value.Digest))
	require.False(t, hasher.Equal(raw, "not-a-fingerprint"))
}

func TestHasher_Deterministic(t *testing.T) {
	t.Parallel()

	hasher := sha256fingerprint.New()
	first, err := hasher.Hash("same")
	require.NoError(t, err)

	second, err := hasher.Hash("same")
	require.NoError(t, err)
	require.Equal(t, first, second)
}
