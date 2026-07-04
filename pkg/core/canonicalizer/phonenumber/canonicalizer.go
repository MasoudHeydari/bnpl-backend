package phonenumber

import "github.com/parstis/bnpl-backend/pkg/core/model"

// Canonicalizer validates and canonicalizes a phone number according to the
// specified jurisdiction.
//
// Implementations are local adapters. They must not perform network I/O.
// This is not phone-possession verification (OTP); it only performs syntax and
// jurisdiction-specific validation plus canonicalization.
//
// Error contract: implementations must never include the raw phone value in
// returned error messages (PII). Prefer the package sentinel errors
// (ErrRequired, ErrInvalid, ErrUnsupportedJurisdiction). Callers still sanitize
// unknown errors as defense in depth.
type Canonicalizer interface {
	Canonicalize(
		jurisdiction model.Jurisdiction,
		raw string,
	) (string, error)
}
