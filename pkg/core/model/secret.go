package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Secret holds a sensitive string with redacted String, JSON, and YAML output.
// Plaintext is available only through Reveal, which should be used solely for
// connection setup and never logged.
type Secret struct {
	// value holds the plaintext secret. It is unexported so callers must go
	// through Reveal, and is never included in String, Format, JSON, or YAML
	// output.
	value string

	// hash is the SHA-256 hex digest of value, used for redacted display and
	// equality comparisons without exposing the plaintext.
	hash string
}

// NewSecret constructs a Secret from a raw string value.
func NewSecret(value string) Secret {
	hash := sha256.Sum256([]byte(value))
	return Secret{
		value: value,
		hash:  hex.EncodeToString(hash[:]),
	}
}

// UnmarshalYAML decodes a YAML string into Secret, storing a SHA-256 hash for comparisons.
func (s *Secret) UnmarshalYAML(value *yaml.Node) error {
	var str string
	if err := value.Decode(&str); err != nil {
		return fmt.Errorf("failed to decode secret: %w", err)
	}

	// Store the raw value
	s.value = str

	// Calculate and store SHA256 hash
	hash := sha256.Sum256([]byte(str))
	s.hash = hex.EncodeToString(hash[:])

	return nil
}

// MarshalYAML returns a redacted representation; it never writes the raw value.
func (s Secret) MarshalYAML() (interface{}, error) {
	if s.value == "" && s.hash == "" {
		return "", nil
	}

	// Never output the actual value, only a redacted version
	if s.hash != "" {
		return fmt.Sprintf("REDACTED[%s...]", s.hash[:16]), nil
	}

	return "REDACTED", nil
}

// String returns a redacted form safe for logs and fmt printing.
func (s Secret) String() string {
	if s.hash == "" {
		return ""
	}

	return fmt.Sprintf("Secret{%s...}", s.hash[:16])
}

// GoString returns a redacted form for %#v formatting.
func (s Secret) GoString() string {
	return s.String()
}

// Format implements redacted custom formatting for fmt verbs.
func (s Secret) Format(f fmt.State, verb rune) {
	switch verb {
	case 'v':
		if f.Flag('#') {
			_, _ = fmt.Fprintf(f, "model.Secret{hash: %s...}", s.hash[:16])
		} else if f.Flag('+') {
			_, _ = fmt.Fprintf(f, "Secret{hash: %s, length: %d}", s.hash[:16], len(s.value))
		} else {
			_, _ = fmt.Fprint(f, s.String())
		}
	case 's':
		_, _ = fmt.Fprint(f, s.String())
	default:
		_, _ = fmt.Fprint(f, s.String())
	}
}

// Reveal returns the plaintext secret for connection setup.
// Do not log or persist the returned value; callers must treat it as sensitive.
func (s Secret) Reveal() string {
	return s.value
}

// Hash returns the SHA-256 hex digest of the secret value.
func (s Secret) Hash() string {
	return s.hash
}

// Verify reports whether password matches the stored secret by comparing SHA-256 hashes.
func (s Secret) Verify(password string) bool {
	hash := sha256.Sum256([]byte(password))
	return s.hash == hex.EncodeToString(hash[:])
}

// IsEmpty reports whether the secret has no stored value.
func (s Secret) IsEmpty() bool {
	return s.value == "" && s.hash == ""
}

// Clear removes the stored value and hash. Reveal returns empty afterward.
func (s *Secret) Clear() {
	s.value = ""
	s.hash = ""
}

// Equal reports whether two secrets have the same value without revealing either.
func (s Secret) Equal(other Secret) bool {
	return s.hash == other.hash
}

// UnmarshalJSON decodes a JSON string into Secret.
func (s *Secret) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return fmt.Errorf("failed to decode JSON secret: %w", err)
	}

	s.value = str
	hash := sha256.Sum256([]byte(str))
	s.hash = hex.EncodeToString(hash[:])

	return nil
}

// MarshalJSON returns a redacted JSON string; it never writes the raw value.
func (s Secret) MarshalJSON() ([]byte, error) {
	if s.value == "" && s.hash == "" {
		return json.Marshal("")
	}

	// Never output the actual value in JSON
	if s.hash != "" {
		return json.Marshal(fmt.Sprintf("REDACTED[%s...]", s.hash[:16]))
	}

	return json.Marshal("REDACTED")
}
