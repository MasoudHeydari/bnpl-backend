package merchantterminal

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// APIKeyPrefix is the fixed prefix of every generated API key.
	APIKeyPrefix = "bnpl_"

	// apiKeyRandomBytes is the number of random bytes behind an API key.
	apiKeyRandomBytes = 32
)

// ErrInvalidAPIKeyFormat is returned when an API key does not have the shape
// produced by GenerateAPIKey. Callers must not reveal which check failed.
var ErrInvalidAPIKeyFormat = errors.New("api key format is invalid")

// GenerateAPIKey returns a new API key of the form "bnpl_" followed by the
// unpadded base64url encoding of 32 bytes read from r. Pass
// crypto/rand.Reader in production.
func GenerateAPIKey(r io.Reader) (string, error) {
	const op = "merchantterminal.GenerateAPIKey"

	buf := make([]byte, apiKeyRandomBytes)

	_, err := io.ReadFull(r, buf)
	if err != nil {
		return "", fmt.Errorf("%s: read random: %w", op, err)
	}

	return APIKeyPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// ValidateAPIKeyFormat reports whether apiKey has the exact shape produced by
// GenerateAPIKey. It does not verify that the key is issued.
func ValidateAPIKeyFormat(apiKey string) error {
	encoded, ok := strings.CutPrefix(apiKey, APIKeyPrefix)
	if !ok {
		return ErrInvalidAPIKeyFormat
	}

	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return ErrInvalidAPIKeyFormat
	}

	if len(raw) != apiKeyRandomBytes {
		return ErrInvalidAPIKeyFormat
	}

	return nil
}
