package model

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const (
	// CheckoutSessionTokenPrefix marks a public checkout session token.
	CheckoutSessionTokenPrefix = "cs_"

	// CheckoutSessionTokenRandomBytes is the amount of entropy in a token.
	CheckoutSessionTokenRandomBytes = 32

	// CheckoutSessionTokenLength is the total length of a well-formed token:
	// the prefix followed by the lowercase hex encoding of the random bytes.
	CheckoutSessionTokenLength = len(CheckoutSessionTokenPrefix) + 2*CheckoutSessionTokenRandomBytes
)

// ErrInvalidCheckoutSessionToken reports a value that is not a well-formed
// public checkout session token.
var ErrInvalidCheckoutSessionToken = errors.New("invalid checkout session token")

// CheckoutSessionToken is the public, unguessable handle for a checkout session
// (checkout_sessions.public_token).
//
// It is the only checkout identifier shared with merchants and browsers. The
// format is "cs_" followed by 64 lowercase hex characters (32 random bytes).
// The internal CheckoutSessionID never crosses that boundary.
type CheckoutSessionToken string

// GenerateCheckoutSessionToken draws a new token from r. Production callers
// must pass a cryptographically secure source such as crypto/rand.Reader.
func GenerateCheckoutSessionToken(r io.Reader) (CheckoutSessionToken, error) {
	const op = "model.GenerateCheckoutSessionToken"

	if r == nil {
		return "", fmt.Errorf("%s: random source is required", op)
	}

	var raw [CheckoutSessionTokenRandomBytes]byte
	if _, err := io.ReadFull(r, raw[:]); err != nil {
		return "", fmt.Errorf("%s: read random bytes: %w", op, err)
	}

	return CheckoutSessionToken(CheckoutSessionTokenPrefix + hex.EncodeToString(raw[:])), nil
}

// ParseCheckoutSessionToken validates value and returns it as a token.
// No normalization is applied: upper-case hex or surrounding whitespace is rejected.
func ParseCheckoutSessionToken(value string) (CheckoutSessionToken, error) {
	token := CheckoutSessionToken(value)
	if err := token.Validate(); err != nil {
		return "", err
	}

	return token, nil
}

// Validate reports whether t has the canonical "cs_<64 lowercase hex>" format.
func (t CheckoutSessionToken) Validate() error {
	value := string(t)
	if len(value) != CheckoutSessionTokenLength {
		return ErrInvalidCheckoutSessionToken
	}

	if value[:len(CheckoutSessionTokenPrefix)] != CheckoutSessionTokenPrefix {
		return ErrInvalidCheckoutSessionToken
	}

	for _, c := range value[len(CheckoutSessionTokenPrefix):] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ErrInvalidCheckoutSessionToken
		}
	}

	return nil
}

// String returns the token text.
func (t CheckoutSessionToken) String() string { return string(t) }

// IsZero reports whether t is empty.
func (t CheckoutSessionToken) IsZero() bool { return t == "" }
