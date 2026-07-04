package fingerprint

import "errors"

// ErrPepperKeyMissing is returned by Verify when a stored verifier references a
// pepper key id that is not present in the configured key ring.
var ErrPepperKeyMissing = errors.New("pepper key is missing")

// Hasher hashes raw material into a self-describing Value and verifies stored
// digests.
//
// Implementations must be deterministic for a given configuration so a
// presented value can be re-hashed and compared. They must never expose the
// plaintext raw value.
//
// The storable form of a digest is Value.String() (algorithm[:key_id]:digest).
// Pass that same form to Equal / Verify as hash.
//
// Concrete algorithms live in infrastructure adapters. Callers choose the
// algorithm at wiring time; this port stays algorithm-agnostic.
type Hasher interface {
	// Hash returns the storable digest of raw.
	Hash(raw string) (Value, error)

	// Verify reports whether raw matches hash.
	//
	// A false result with a nil error is a normal credential mismatch (or a
	// malformed verifier). A non-nil error is an infrastructure / configuration
	// failure such as a missing pepper key referenced by the stored verifier.
	// Public authentication APIs must still map such errors to a generic
	// unauthorized response without leaking the reason.
	Verify(raw, hash string) (bool, error)

	// Equal reports whether raw matches hash. It is equivalent to ignoring the
	// error from Verify: configuration failures look like a mismatch. Prefer
	// Verify when the caller must distinguish those cases for internal
	// observability.
	Equal(raw, hash string) bool
}
