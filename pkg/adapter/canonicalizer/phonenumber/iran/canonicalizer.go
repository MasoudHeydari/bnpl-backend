package iran

import "github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"

const (
	localPrefix      = "09"
	e164Prefix       = "+989"
	subscriberDigits = 9
)

// Canonicalizer validates and canonicalizes Iranian mobile numbers.
type Canonicalizer struct{}

// New returns an Iranian phone canonicalizer.
func New() *Canonicalizer {
	return &Canonicalizer{}
}

// Canonicalize accepts an Iranian mobile in exactly one of two forms and
// returns its canonical E.164 representation:
//
//	09xxxxxxxxx    -> +989xxxxxxxxx
//	+989xxxxxxxxx  -> +989xxxxxxxxx
//
// The raw value is not trimmed or otherwise normalized.
// Returned errors must never include the raw phone value (PII).
func (c *Canonicalizer) Canonicalize(raw string) (string, error) {
	if raw == "" {
		return "", phonenumber.ErrRequired
	}

	var subscriber string

	switch {
	case len(raw) == len(localPrefix)+subscriberDigits &&
		raw[:len(localPrefix)] == localPrefix:
		subscriber = raw[len(localPrefix):]

	case len(raw) == len(e164Prefix)+subscriberDigits &&
		raw[:len(e164Prefix)] == e164Prefix:
		subscriber = raw[len(e164Prefix):]

	default:
		return "", phonenumber.ErrInvalid
	}

	if !isASCIIDigits(subscriber) {
		return "", phonenumber.ErrInvalid
	}

	return e164Prefix + subscriber, nil
}

func isASCIIDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}

	return true
}
