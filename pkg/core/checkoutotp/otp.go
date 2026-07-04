package checkoutotp

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"strconv"
)

const (
	// Digits is the number of decimal digits in a checkout OTP.
	Digits = 6

	// otpUpperBound is the exclusive upper bound of a six-digit code (10^6).
	otpUpperBound = 1_000_000

	// materialDomain separates checkout OTP hashes from any other HMAC use of
	// the same key. Bump the version when the material layout changes.
	materialDomain = "bnpl:checkout-otp:v2"

	// phoneRateLimitDomain separates per-phone OTP rate-limit fingerprints from
	// OTP verifiers and any other HMAC use of the same pepper ring.
	phoneRateLimitDomain = "bnpl:checkout-otp-rate-limit:phone:v1"
)

// GenerateSixDigitOTP draws a uniformly distributed code in [000000, 999999]
// from r and returns it zero-padded. Production callers must pass a
// cryptographically secure source such as crypto/rand.Reader.
func GenerateSixDigitOTP(r io.Reader) (string, error) {
	const op = "checkoutotp.GenerateSixDigitOTP"

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

	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}

	return true
}

// DomainSeparatedMaterial returns the byte string an OTP verifier HMAC is
// computed over. Binding the public checkout token into the material makes a
// verifier valid for exactly one checkout session, and binding the evaluation
// generation makes it valid only for the offer terms it was issued for. The
// separator cannot occur inside a well-formed token, generation, or code.
func DomainSeparatedMaterial(token string, evaluationGeneration int, otp string) string {
	return materialDomain + "|" + token + "|" + strconv.Itoa(evaluationGeneration) + "|" + otp
}

// PhoneRateLimitMaterial returns the byte string hashed into the durable
// per-phone OTP rate-limit key. The raw phone must never be stored as the
// rate-limit key; callers HMAC this material with the project's keyed
// fingerprint hasher. Do not log the phone or this material.
func PhoneRateLimitMaterial(phoneE164 string) string {
	return phoneRateLimitDomain + "|" + phoneE164
}
