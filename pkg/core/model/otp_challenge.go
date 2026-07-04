package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OTPPurpose identifies which application capability an OTP challenge
// authorizes. Purposes are closed; callers must not invent arbitrary strings.
type OTPPurpose string

const (
	// OTPPurposeHostedCheckoutConfirm authorizes hosted checkout confirmation
	// (initial offer confirm or payment-continuation confirm) for one subject.
	OTPPurposeHostedCheckoutConfirm OTPPurpose = "hosted_checkout_confirm"
)

// IsValid reports whether purpose is a known OTP purpose.
func (p OTPPurpose) IsValid() bool {
	switch p {
	case OTPPurposeHostedCheckoutConfirm:
		return true

	default:
		return false
	}
}

// OTPChallengeID uniquely identifies an OTPChallenge row.
type OTPChallengeID uuid.UUID

// ParseOTPChallengeID parses an OTPChallengeID from its canonical string form.
func ParseOTPChallengeID(value string) (OTPChallengeID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return OTPChallengeID{}, fmt.Errorf("parse otp challenge id: %w", err)
	}

	return OTPChallengeID(id), nil
}

func (id OTPChallengeID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id OTPChallengeID) String() string { return uuid.UUID(id).String() }

func (id OTPChallengeID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id OTPChallengeID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *OTPChallengeID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = OTPChallengeID(u)

	return nil
}

func (id *OTPChallengeID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = OTPChallengeID(u)

	return nil
}

func (id OTPChallengeID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// OTPChallenge is the durable state of one purpose/subject OTP scope.
//
// Plaintext codes are never stored. Only a keyed verifier of domain-separated
// material is persisted. IssueID scopes Notification acceptance and definite-
// rejection cleanup so a stale SMS sender cannot mutate a newer challenge.
type OTPChallenge struct {
	// ID is the internal primary key.
	ID OTPChallengeID `db:"id" json:"id"`

	// Purpose is the application capability this challenge authorizes.
	Purpose OTPPurpose `db:"purpose" json:"purpose"`

	// SubjectKey is the opaque application subject identity for this purpose
	// (for hosted checkout, the CheckoutSession ID text).
	SubjectKey string `db:"subject_key" json:"subject_key"`

	// ContextVersion is an opaque application binding (for hosted checkout,
	// the evaluation generation as decimal text). Empty when no challenge has
	// been issued yet.
	ContextVersion string `db:"context_version" json:"context_version"`

	// IssueID is the opaque identity of the currently issued challenge.
	IssueID *OTPIssueID `db:"issue_id" json:"-"`

	// Verifier is the keyed hash of the currently issued OTP.
	Verifier *string `db:"verifier" json:"-"`

	// ExpiresAt is when the currently issued OTP stops being accepted.
	ExpiresAt *time.Time `db:"expires_at" json:"-"`

	// NextIssueAt is the durable earliest time another OTP may be issued.
	// Set to the original issuance-window deadline and preserved across
	// successful verification.
	NextIssueAt *time.Time `db:"next_issue_at" json:"-"`

	// NotificationAcceptedAt is when Notification durably accepted submission
	// of the currently issued OTP.
	NotificationAcceptedAt *time.Time `db:"notification_accepted_at" json:"-"`

	// VerifiedAt records when the customer successfully consumed an OTP.
	VerifiedAt *time.Time `db:"verified_at" json:"-"`

	// AttemptCount counts failed verification attempts for the current OTP.
	AttemptCount int `db:"attempt_count" json:"-"`

	// NotificationRequestID is the Notification Service request identifier for
	// the accepted OTP submission.
	NotificationRequestID *string `db:"notification_request_id" json:"-"`

	// SendCount is the number of one-time codes issued for this subject/purpose.
	// It is never reset and enforces the per-subject send cap.
	SendCount int `db:"send_count" json:"-"`

	// CreatedAt is when the challenge row was first ensured.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the challenge row was last mutated.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
