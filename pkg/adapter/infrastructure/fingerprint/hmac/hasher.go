package hmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
)

// MinPepperLength is the shortest accepted pepper, in bytes.
const MinPepperLength = 32

// ErrPepperTooShort is returned when a pepper is shorter than MinPepperLength.
var ErrPepperTooShort = errors.New("pepper is too short")

// ErrPepperKeyMissing aliases fingerprint.ErrPepperKeyMissing for callers that
// import the HMAC adapter package.
var ErrPepperKeyMissing = fingerprint.ErrPepperKeyMissing

// Hasher implements fingerprint.Hasher with HMAC-SHA256 over a versioned pepper
// key ring.
//
// New credentials are hashed with ActiveKeyID. Verification uses the pepper key
// id embedded in the stored verifier (or LegacyUnversionedPepperKeyID for
// pre-ring digests). Missing referenced peppers fail closed — there is no
// fallback to another pepper.
type Hasher struct {
	activeKeyID string
	peppers     map[string][]byte
}

var _ fingerprint.Hasher = (*Hasher)(nil)

// New returns a single-pepper Hasher registered as LegacyUnversionedPepperKeyID
// ("v1"). Prefer NewKeyRing when configuring multiple peppers.
func New(pepper []byte) (*Hasher, error) {
	return NewKeyRing(fingerprint.LegacyUnversionedPepperKeyID, map[string][]byte{
		fingerprint.LegacyUnversionedPepperKeyID: pepper,
	})
}

// NewKeyRing returns a Hasher keyed by peppers. activeKeyID and every map key
// must pass fingerprint.ValidatePepperKeyID. Every pepper must be at least
// MinPepperLength bytes and is copied.
func NewKeyRing(activeKeyID string, peppers map[string][]byte) (*Hasher, error) {
	const op = "hmac.NewKeyRing"

	if err := fingerprint.ValidatePepperKeyID(activeKeyID); err != nil {
		return nil, fmt.Errorf("%s: active pepper key id: %w", op, err)
	}

	if len(peppers) == 0 {
		return nil, fmt.Errorf("%s: at least one pepper key is required", op)
	}

	if _, ok := peppers[activeKeyID]; !ok {
		return nil, fmt.Errorf("%s: active pepper key id %q is not in the key ring", op, activeKeyID)
	}

	copied := make(map[string][]byte, len(peppers))

	for id, pepper := range peppers {
		if err := fingerprint.ValidatePepperKeyID(id); err != nil {
			return nil, fmt.Errorf("%s: pepper key id: %w", op, err)
		}

		if len(pepper) < MinPepperLength {
			return nil, fmt.Errorf(
				"%s: pepper key %q needs at least %d bytes: %w",
				op,
				id,
				MinPepperLength,
				ErrPepperTooShort,
			)
		}

		copied[id] = append([]byte(nil), pepper...)
	}

	return &Hasher{activeKeyID: activeKeyID, peppers: copied}, nil
}

// Hash returns the storable digest of raw using the active pepper key.
//
// Format: hmac-sha256:<active_key_id>:<hex_digest>
func (h *Hasher) Hash(raw string) (fingerprint.Value, error) {
	const op = "hmac.Hasher.Hash"

	pepper, ok := h.peppers[h.activeKeyID]
	if !ok {
		return fingerprint.Value{}, fmt.Errorf("%s: %w", op, ErrPepperKeyMissing)
	}

	value := fingerprint.Value{
		Algorithm: fingerprint.HMACSHA256Algorithm,
		KeyID:     h.activeKeyID,
		Digest:    hex.EncodeToString(sum(pepper, raw)),
	}

	if err := value.Validate(); err != nil {
		return fingerprint.Value{}, fmt.Errorf("%s: %w", op, err)
	}

	return value, nil
}

// Verify reports whether raw matches hash.
//
// hash must be the storable form returned by Value.String(). Verification uses
// only the pepper identified by the stored key id (or v1 for legacy
// unversioned digests). Malformed hashes return (false, nil). A missing
// referenced pepper returns (false, ErrPepperKeyMissing).
func (h *Hasher) Verify(raw, hash string) (bool, error) {
	const op = "hmac.Hasher.Verify"

	value, err := fingerprint.ParseValue(hash)
	if err != nil || value.Algorithm != fingerprint.HMACSHA256Algorithm {
		// Malformed / wrong-algorithm verifiers are credential mismatches.
		return false, nil //nolint:nilerr // parse failures are mismatches, not infra errors
	}

	keyID := value.KeyID
	if keyID == "" {
		keyID = fingerprint.LegacyUnversionedPepperKeyID
	}

	pepper, ok := h.peppers[keyID]
	if !ok {
		return false, fmt.Errorf("%s: pepper key id %q: %w", op, keyID, ErrPepperKeyMissing)
	}

	digest := hex.EncodeToString(sum(pepper, raw))

	var expected string
	if value.KeyID == "" {
		expected = fingerprint.HMACSHA256Algorithm + ":" + digest
	} else {
		expected = fingerprint.Value{
			Algorithm: fingerprint.HMACSHA256Algorithm,
			KeyID:     keyID,
			Digest:    digest,
		}.String()
	}

	if len(expected) != len(hash) {
		return false, nil
	}

	return subtle.ConstantTimeCompare([]byte(expected), []byte(hash)) == 1, nil
}

// Equal reports whether raw matches hash, treating configuration failures the
// same as a mismatch. Prefer Verify when missing-pepper must be observable.
func (h *Hasher) Equal(raw, hash string) bool {
	ok, _ := h.Verify(raw, hash)

	return ok
}

func sum(pepper []byte, raw string) []byte {
	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(raw))

	return mac.Sum(nil)
}
