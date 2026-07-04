package model

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

const (
	// OTPIssueIDPrefix marks an opaque checkout OTP issuance identity.
	OTPIssueIDPrefix = "oi_"

	// OTPIssueIDRandomBytes is the entropy in an OTP issuance identity.
	OTPIssueIDRandomBytes = 16

	otpIssueIDLength = len(OTPIssueIDPrefix) + 2*OTPIssueIDRandomBytes
)

// ErrInvalidOTPIssueID reports a value that is not a well-formed OTP issue id.
var ErrInvalidOTPIssueID = errors.New("invalid otp issue id")

// OTPIssueID is the opaque identity of one OTP issuance
// (otp_challenges.issue_id).
//
// Each Issue call creates a new identity. MarkNotificationAccepted and
// definite-rejection-failure cleanup are compare-and-swap scoped to this id so
// a stale SMS sender cannot mutate a newer challenge. It is never exposed on
// public APIs.
type OTPIssueID string

// GenerateOTPIssueID draws a new issuance identity from r. Production callers
// must pass a cryptographically secure source such as crypto/rand.Reader.
func GenerateOTPIssueID(r io.Reader) (OTPIssueID, error) {
	const op = "model.GenerateOTPIssueID"

	if r == nil {
		return "", fmt.Errorf("%s: random source is required", op)
	}

	var raw [OTPIssueIDRandomBytes]byte
	if _, err := io.ReadFull(r, raw[:]); err != nil {
		return "", fmt.Errorf("%s: read random bytes: %w", op, err)
	}

	return OTPIssueID(OTPIssueIDPrefix + hex.EncodeToString(raw[:])), nil
}

// Validate reports whether id has the canonical "oi_<32 lowercase hex>" format.
func (id OTPIssueID) Validate() error {
	value := string(id)
	if len(value) != otpIssueIDLength {
		return ErrInvalidOTPIssueID
	}

	if value[:len(OTPIssueIDPrefix)] != OTPIssueIDPrefix {
		return ErrInvalidOTPIssueID
	}

	for _, c := range value[len(OTPIssueIDPrefix):] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ErrInvalidOTPIssueID
		}
	}

	return nil
}

// String returns the opaque identity text.
func (id OTPIssueID) String() string { return string(id) }

// IsZero reports whether id is empty.
func (id OTPIssueID) IsZero() bool { return id == "" }
