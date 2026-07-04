package sha256

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"

	corefingerprint "github.com/parstis/bnpl-backend/pkg/core/fingerprint"
)

// Hasher implements fingerprint.Hasher with unkeyed SHA-256.
type Hasher struct{}

// New constructs an unkeyed SHA-256 fingerprint hasher.
func New() *Hasher {
	return &Hasher{}
}

var _ corefingerprint.Hasher = (*Hasher)(nil)

// Hash returns the storable digest of raw.
func (h *Hasher) Hash(raw string) (corefingerprint.Value, error) {
	sum := sha256.Sum256([]byte(raw))

	return corefingerprint.Value{
		Algorithm: corefingerprint.SHA256Algorithm,
		Digest:    hex.EncodeToString(sum[:]),
	}, nil
}

// Verify reports whether raw matches hash.
//
// hash must be the storable form returned by Value.String(). Malformed or
// algorithm-mismatched hashes return (false, nil). Unkeyed SHA-256 has no
// pepper ring, so Verify never returns ErrPepperKeyMissing.
func (h *Hasher) Verify(raw, hash string) (bool, error) {
	value, err := h.Hash(raw)
	if err != nil {
		// Hash failures for this unkeyed hasher are treated as mismatches.
		return false, nil //nolint:nilerr // hash failures are mismatches, not infra errors
	}

	expected := value.String()
	if len(expected) != len(hash) {
		return false, nil
	}

	return subtle.ConstantTimeCompare([]byte(expected), []byte(hash)) == 1, nil
}

// Equal reports whether raw matches hash.
//
// hash must be the storable form returned by Value.String(). Malformed or
// algorithm-mismatched hashes never match.
func (h *Hasher) Equal(raw, hash string) bool {
	ok, _ := h.Verify(raw, hash)

	return ok
}
