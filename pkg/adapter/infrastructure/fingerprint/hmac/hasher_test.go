package hmac_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	fingerprinthmac "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/hmac"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
)

func TestNew_RejectsShortPepper(t *testing.T) {
	t.Parallel()

	_, err := fingerprinthmac.New([]byte("too-short"))
	require.ErrorIs(t, err, fingerprinthmac.ErrPepperTooShort)
}

func TestHasher_HashAndEqual(t *testing.T) {
	t.Parallel()

	pepper := []byte("0123456789abcdef0123456789abcdef")
	hasher, err := fingerprinthmac.New(pepper)
	require.NoError(t, err)

	raw := "bnpl_test-secret-material"
	value, err := hasher.Hash(raw)
	require.NoError(t, err)
	require.NoError(t, value.Validate())
	require.Equal(t, fingerprint.HMACSHA256Algorithm, value.Algorithm)
	require.Equal(t, fingerprint.LegacyUnversionedPepperKeyID, value.KeyID)

	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(raw))
	require.Equal(t, hex.EncodeToString(mac.Sum(nil)), value.Digest)
	require.Equal(
		t,
		fingerprint.HMACSHA256Algorithm+":"+fingerprint.LegacyUnversionedPepperKeyID+":"+value.Digest,
		value.String(),
	)

	require.True(t, hasher.Equal(raw, value.String()))
	require.False(t, hasher.Equal("other", value.String()))
	require.False(t, hasher.Equal(raw, value.Digest))
	require.False(t, hasher.Equal(raw, "not-a-fingerprint"))

	// Legacy unversioned digests still verify against the v1 pepper.
	legacy := fingerprint.HMACSHA256Algorithm + ":" + value.Digest
	require.True(t, hasher.Equal(raw, legacy))
}

func TestHasher_KeyRingVersions(t *testing.T) {
	t.Parallel()

	v1 := []byte("pepper-v1-0123456789abcdef01234567")
	v2 := []byte("pepper-v2-0123456789abcdef01234567")

	ring, err := fingerprinthmac.NewKeyRing("v2", map[string][]byte{"v1": v1, "v2": v2})
	require.NoError(t, err)

	raw := "bnpl_key"
	active, err := ring.Hash(raw)
	require.NoError(t, err)
	require.Equal(t, "v2", active.KeyID)
	require.True(t, ring.Equal(raw, active.String()))

	v1Only, err := fingerprinthmac.NewKeyRing("v1", map[string][]byte{"v1": v1})
	require.NoError(t, err)

	legacy, err := v1Only.Hash(raw)
	require.NoError(t, err)
	require.Equal(t, "v1", legacy.KeyID)
	require.True(t, ring.Equal(raw, legacy.String()))

	v2Only, err := fingerprinthmac.NewKeyRing("v2", map[string][]byte{"v2": v2})
	require.NoError(t, err)
	require.False(t, v2Only.Equal(raw, legacy.String()))

	ok, err := v2Only.Verify(raw, legacy.String())
	require.False(t, ok)
	require.ErrorIs(t, err, fingerprinthmac.ErrPepperKeyMissing)
}

func TestNewKeyRing_RejectsInvalidKeyIDs(t *testing.T) {
	t.Parallel()

	pepper := []byte("0123456789abcdef0123456789abcdef")

	_, err := fingerprinthmac.NewKeyRing("prod:v2", map[string][]byte{"prod:v2": pepper})
	require.Error(t, err)

	_, err = fingerprinthmac.NewKeyRing(" v2 ", map[string][]byte{" v2 ": pepper})
	require.Error(t, err)

	_, err = fingerprinthmac.NewKeyRing("v2", map[string][]byte{"bad:id": pepper, "v2": pepper})
	require.Error(t, err)

	_, err = fingerprinthmac.NewKeyRing("missing", map[string][]byte{"v2": pepper})
	require.Error(t, err)
}

func TestHasher_CopiesPepper(t *testing.T) {
	t.Parallel()

	pepper := []byte("0123456789abcdef0123456789abcdef")
	hasher, err := fingerprinthmac.New(pepper)
	require.NoError(t, err)

	raw := "value"
	before, err := hasher.Hash(raw)
	require.NoError(t, err)

	pepper[0] ^= 0xff

	after, err := hasher.Hash(raw)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
