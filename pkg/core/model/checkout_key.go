package model

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

const (
	// MaxCheckoutKeyLength is the maximum accepted length for a canonical checkout key.
	MaxCheckoutKeyLength = 256
)

// NormalizeCheckoutKey trims and validates the canonical checkout identity derived from
// the HTTP Idempotency-Key header.
func NormalizeCheckoutKey(key string) (string, error) {
	normalized := strings.TrimSpace(key)
	if normalized == "" {
		return "", errors.New("checkout key is required")
	}

	if len(normalized) > MaxCheckoutKeyLength {
		return "", errors.New("checkout key exceeds maximum length")
	}

	return normalized, nil
}

// CheckoutKeyTargetID derives a stable decision target id from the canonical checkout key.
func CheckoutKeyTargetID(checkoutKey string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("checkout:"+checkoutKey))
}
