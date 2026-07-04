package fingerprint

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// SHA256Algorithm identifies the unkeyed SHA-256 digest algorithm.
	SHA256Algorithm = "sha256"

	// HMACSHA256Algorithm identifies the peppered HMAC-SHA256 digest algorithm.
	HMACSHA256Algorithm = "hmac-sha256"

	// hexDigestLength is the length of a lowercase hex SHA-256 digest.
	hexDigestLength = 64

	// LegacyUnversionedPepperKeyID is the pepper key id used when verifying
	// legacy digests stored as "hmac-sha256:<digest>" without an embedded key
	// id (pre-key-ring issuances).
	LegacyUnversionedPepperKeyID = "v1"
)

// Value is a self-describing digest.
//
// Storable forms:
//   - sha256:<digest>
//   - hmac-sha256:<digest>                         (legacy, unversioned)
//   - hmac-sha256:<pepper_key_id>:<digest>         (versioned pepper)
type Value struct {
	// Algorithm identifies the hash algorithm (for example "sha256").
	Algorithm string

	// KeyID identifies the pepper key used for HMAC digests. Empty for SHA-256
	// and for legacy unversioned HMAC digests.
	KeyID string

	// Digest is the lowercase hexadecimal digest produced by Algorithm.
	Digest string
}

// String returns the self-describing storable representation.
func (v Value) String() string {
	if v.Algorithm == "" || v.Digest == "" {
		return ""
	}

	if v.KeyID != "" {
		return v.Algorithm + ":" + v.KeyID + ":" + v.Digest
	}

	return v.Algorithm + ":" + v.Digest
}

// ParseValue parses a storable fingerprint string into a Value.
func ParseValue(raw string) (Value, error) {
	const op = "fingerprint.ParseValue"

	parts := strings.Split(raw, ":")
	switch len(parts) {
	case 2:
		value := Value{Algorithm: parts[0], Digest: parts[1]}
		if err := value.Validate(); err != nil {
			return Value{}, fmt.Errorf("%s: %w", op, err)
		}

		return value, nil

	case 3:
		if parts[0] != HMACSHA256Algorithm {
			return Value{}, fmt.Errorf("%s: key id is only valid for %s", op, HMACSHA256Algorithm)
		}

		value := Value{Algorithm: parts[0], KeyID: parts[1], Digest: parts[2]}
		if err := value.Validate(); err != nil {
			return Value{}, fmt.Errorf("%s: %w", op, err)
		}

		return value, nil

	default:
		return Value{}, fmt.Errorf("%s: malformed fingerprint", op)
	}
}

// Validate reports whether the value is usable for persistence.
//
// It enforces a known algorithm and digest representation so a broken or
// future adapter cannot cause an empty or malformed digest to be persisted.
func (v Value) Validate() error {
	const op = "fingerprint.Value.Validate"

	if v.Algorithm == "" {
		return fmt.Errorf("%s: fingerprint algorithm is required", op)
	}

	switch v.Algorithm {
	case SHA256Algorithm:
		if v.KeyID != "" {
			return fmt.Errorf("%s: key id is not valid for %s", op, SHA256Algorithm)
		}

	case HMACSHA256Algorithm:

	default:
		return fmt.Errorf("%s: unsupported fingerprint algorithm %q", op, v.Algorithm)
	}

	if v.KeyID != "" {
		if err := validatePepperKeyID(v.KeyID); err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	if v.Digest == "" {
		return fmt.Errorf("%s: fingerprint digest is required", op)
	}

	if len(v.Digest) != hexDigestLength {
		return fmt.Errorf("%s: fingerprint digest must be %d hexadecimal characters", op, hexDigestLength)
	}

	for _, r := range v.Digest {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return fmt.Errorf("%s: fingerprint digest must be lowercase hexadecimal", op)
		}
	}

	if v.String() == "" {
		return fmt.Errorf("%s: fingerprint string is empty", op)
	}

	return nil
}

// pepperKeyIDPattern is the canonical grammar for versioned pepper key ids
// embedded in hmac-sha256:<key_id>:<digest>. Key ids must not contain ':' so
// ParseValue can split the verifier unambiguously.
var pepperKeyIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidatePepperKeyID reports whether keyID is a canonical pepper key id.
//
// Accepted form: 1..64 characters from [A-Za-z0-9_-]. Empty, whitespace, and
// ':' are rejected so a configured key id always round-trips through
// Value.String / ParseValue.
func ValidatePepperKeyID(keyID string) error {
	if keyID == "" {
		return fmt.Errorf("pepper key id is required")
	}

	if !pepperKeyIDPattern.MatchString(keyID) {
		return fmt.Errorf(
			"pepper key id %q must match %s",
			keyID,
			pepperKeyIDPattern.String(),
		)
	}

	return nil
}

func validatePepperKeyID(keyID string) error {
	return ValidatePepperKeyID(keyID)
}
