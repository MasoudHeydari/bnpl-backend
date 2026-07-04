package otpcode

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
)

const (
	// Digits is the number of decimal digits in a checkout OTP.
	Digits = 6

	// otpUpperBound is the exclusive upper bound of a six-digit code (10^6).
	otpUpperBound = 1_000_000

	// materialDomain separates OTP hashes from any other HMAC use of the same
	// key. Bump the version when the material layout changes.
	//
	// v2 uses length-prefixed fields so purpose/binding/context/otp boundaries
	// cannot collide under arbitrary future consumer strings.
	materialDomain = "bnpl:otp:v2"

	// phoneRateLimitDomain separates per-phone OTP rate-limit fingerprints from
	// OTP verifiers and any other HMAC use of the same pepper ring.
	phoneRateLimitDomain = "bnpl:otp-rate-limit:phone:v1"
)

// GenerateSixDigitOTP draws a uniformly distributed code in [000000, 999999]
// from r and returns it zero-padded. Production callers must pass a
// cryptographically secure source such as crypto/rand.Reader.
func GenerateSixDigitOTP(r io.Reader) (string, error) {
	const op = "otpcode.GenerateSixDigitOTP"

	if r == nil {
		return "", fmt.Errorf("%s: random source is required", op)
	}

	n, err := rand.Int(r, big.NewInt(otpUpperBound))
	if err != nil {
		return "", fmt.Errorf("%s: draw random code: %w", op, err)
	}

	return fmt.Sprintf("%06d", n.Int64()), nil
}

// IsWellFormed reports whether code is exactly Digits ASCII decimal digits.
func IsWellFormed(code string) bool {
	if len(code) != Digits {
		return false
	}

	for i := 0; i < Digits; i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}

	return true
}

// DomainSeparatedMaterial returns the byte string an OTP verifier HMAC is
// computed over. Binding purpose, subject binding, and context version into the
// material makes a verifier valid for exactly one logical challenge scope.
//
// Fields are length-prefixed (`|<len>:<bytes>`) after the material domain so
// encodings are injective even when consumers supply empty strings or values
// that contain `|` / `:` / digits. Issuer and verifier must use the same
// domain version and encoding.
func DomainSeparatedMaterial(purpose, binding, contextVersion, otp string) string {
	var b strings.Builder
	// Domain + four length-prefixed fields; typical hosted-checkout sizes stay small.
	b.Grow(len(materialDomain) + 48 + len(purpose) + len(binding) + len(contextVersion) + len(otp))
	b.WriteString(materialDomain)
	writeLenPrefixedField(&b, purpose)
	writeLenPrefixedField(&b, binding)
	writeLenPrefixedField(&b, contextVersion)
	writeLenPrefixedField(&b, otp)

	return b.String()
}

func writeLenPrefixedField(b *strings.Builder, field string) {
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(len(field)))
	b.WriteByte(':')
	b.WriteString(field)
}

// PhoneRateLimitMaterial returns the byte string hashed into the durable
// per-phone OTP rate-limit key. The raw phone must never be stored as the
// rate-limit key; callers HMAC this material with the project's keyed
// fingerprint hasher. Do not log the phone or this material.
func PhoneRateLimitMaterial(phoneE164 string) string {
	return phoneRateLimitDomain + "|" + phoneE164
}
