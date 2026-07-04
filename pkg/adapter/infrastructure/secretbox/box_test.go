package secretbox_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/secretbox"
)

var (
	keyA = bytes.Repeat([]byte{0x0a}, secretbox.KeyLength)
	keyB = bytes.Repeat([]byte{0x0b}, secretbox.KeyLength)
)

func newBox(t *testing.T, active string) *secretbox.Box {
	t.Helper()

	box, err := secretbox.New(secretbox.Config{
		ActiveKeyID: active,
		Keys:        map[string][]byte{"k1": keyA, "k2": keyB},
	})
	require.NoError(t, err)

	return box
}

func TestNew_Validation(t *testing.T) {
	t.Parallel()

	tests := map[string]secretbox.Config{
		"no keys":        {ActiveKeyID: "k1"},
		"short key":      {ActiveKeyID: "k1", Keys: map[string][]byte{"k1": []byte("short")}},
		"bad key id":     {ActiveKeyID: "a.b", Keys: map[string][]byte{"a.b": keyA}},
		"missing active": {ActiveKeyID: "k9", Keys: map[string][]byte{"k1": keyA}},
	}

	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := secretbox.New(cfg)
			require.Error(t, err)
		})
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	box := newBox(t, "k1")

	ciphertext, keyID, err := box.Encrypt("whsec_super-secret")
	require.NoError(t, err)
	require.Equal(t, "k1", keyID)
	require.True(t, strings.HasPrefix(ciphertext, "sb1."))
	require.NotContains(t, ciphertext, "whsec_super-secret")

	plaintext, err := box.Decrypt(ciphertext, keyID)
	require.NoError(t, err)
	require.Equal(t, "whsec_super-secret", plaintext)
}

func TestEncrypt_NonceIsRandom(t *testing.T) {
	t.Parallel()

	box := newBox(t, "k1")

	first, _, err := box.Encrypt("same")
	require.NoError(t, err)

	second, _, err := box.Encrypt("same")
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}

func TestEncrypt_RejectsEmptyAndOversized(t *testing.T) {
	t.Parallel()

	box := newBox(t, "k1")

	_, _, err := box.Encrypt("")
	require.Error(t, err)

	_, _, err = box.Encrypt(strings.Repeat("x", 5000))
	require.Error(t, err)
}

func TestDecrypt_RotatedKey(t *testing.T) {
	t.Parallel()

	ciphertext, keyID, err := newBox(t, "k1").Encrypt("secret")
	require.NoError(t, err)

	rotated := newBox(t, "k2")

	plaintext, err := rotated.Decrypt(ciphertext, keyID)
	require.NoError(t, err)
	require.Equal(t, "secret", plaintext)

	_, newKeyID, err := rotated.Encrypt("secret")
	require.NoError(t, err)
	require.Equal(t, "k2", newKeyID)
}

func TestDecrypt_Failures(t *testing.T) {
	t.Parallel()

	box := newBox(t, "k1")

	ciphertext, keyID, err := box.Encrypt("secret")
	require.NoError(t, err)

	flipped := ciphertext[:len(ciphertext)-2] + "AA"
	if flipped == ciphertext {
		flipped = ciphertext[:len(ciphertext)-2] + "BB"
	}

	_, err = box.Decrypt(ciphertext, "missing")
	require.ErrorIs(t, err, secretbox.ErrUnknownKeyID)

	// Wrong (but configured) key id: key and AAD both differ.
	_, err = box.Decrypt(ciphertext, "k2")
	require.ErrorIs(t, err, secretbox.ErrDecryptFailed)

	for name, input := range map[string]string{
		"tampered":     flipped,
		"wrong prefix": "sb2." + strings.TrimPrefix(ciphertext, "sb1."),
		"no prefix":    strings.TrimPrefix(ciphertext, "sb1."),
		"not base64":   "sb1.!!!",
		"too short":    "sb1.AAAA",
		"empty":        "",
	} {
		_, err = box.Decrypt(input, keyID)
		require.ErrorIs(t, err, secretbox.ErrDecryptFailed, name)
	}
}

func TestDecrypt_KeyIDBoundToCiphertext(t *testing.T) {
	t.Parallel()

	// Same key bytes registered under two ids: AAD must still prevent relabeling.
	box, err := secretbox.New(secretbox.Config{
		ActiveKeyID: "k1",
		Keys:        map[string][]byte{"k1": keyA, "alias": keyA},
	})
	require.NoError(t, err)

	ciphertext, _, err := box.Encrypt("secret")
	require.NoError(t, err)

	_, err = box.Decrypt(ciphertext, "alias")
	require.ErrorIs(t, err, secretbox.ErrDecryptFailed)
}
