package phonenumber

import "errors"

var (
	// ErrRequired reports an empty phone number.
	ErrRequired = errors.New("phone number is required")

	// ErrInvalid reports a phone number that fails the jurisdiction's rules.
	ErrInvalid = errors.New("phone number is invalid")

	// ErrUnsupportedJurisdiction reports that the jurisdiction has no
	// registered phone canonicalizer in this binary/deployment wiring.
	// This is adapter availability, not business origination admission
	// (see domaincapability.Jurisdictions).
	ErrUnsupportedJurisdiction = errors.New("phone number jurisdiction is unsupported")
)
