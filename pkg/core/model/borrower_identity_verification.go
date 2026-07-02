package model

import (
	"time"

	"github.com/google/uuid"
)

// BorrowerIdentityVerificationStatus identifies the verification state of a
// borrower's identity verification attempt.
type BorrowerIdentityVerificationStatus string

const (
	// PendingBorrowerIdentityVerificationStatus indicates the verification
	// attempt has been created but has not reached a final result yet.
	PendingBorrowerIdentityVerificationStatus BorrowerIdentityVerificationStatus = "pending"

	// VerifiedBorrowerIdentityVerificationStatus indicates the borrower's
	// identity data was successfully verified.
	VerifiedBorrowerIdentityVerificationStatus BorrowerIdentityVerificationStatus = "verified"

	// RejectedBorrowerIdentityVerificationStatus indicates the borrower's
	// identity data failed verification.
	RejectedBorrowerIdentityVerificationStatus BorrowerIdentityVerificationStatus = "rejected"

	// ExpiredBorrowerIdentityVerificationStatus indicates the verification
	// result is no longer fresh enough for BNPL decisioning.
	ExpiredBorrowerIdentityVerificationStatus BorrowerIdentityVerificationStatus = "expired"
)

// BorrowerIdentityVerification represents a borrower identity verification
// artifact used during BNPL onboarding.
//
// It stores verification evidence and result metadata separately from
// BorrowerProfile so the BNPL profile does not own raw personal identity data.
// Sensitive values must not be exposed through JSON responses.
type BorrowerIdentityVerification struct {
	// ID uniquely identifies this identity verification record.
	ID uuid.UUID `db:"id" json:"id"`

	// BorrowerID references the borrower's identity record.
	BorrowerID uuid.UUID `db:"borrower_id" json:"borrower_id"`

	// NationalIDHash stores a non-reversible hash of the national identifier.
	NationalIDHash string `db:"national_id_hash" json:"-"`

	// PhoneNumberHash stores a non-reversible hash of the phone number.
	PhoneNumberHash string `db:"phone_number_hash" json:"-"`

	// Birthdate is sensitive and must not be exposed through JSON.
	Birthdate time.Time `db:"birthdate" json:"-"`

	// Status is the current result state of this verification attempt.
	Status BorrowerIdentityVerificationStatus `db:"status" json:"status,omitempty"`

	// Provider identifies the verification provider.
	Provider string `db:"provider" json:"provider,omitempty"`

	// ProviderReferenceID references the provider's own tracking identifier.
	ProviderReferenceID *string `db:"provider_reference_id" json:"provider_reference_id,omitempty"`

	// VerifiedAt is set when the verification reaches a successful verified state.
	VerifiedAt *time.Time `db:"verified_at" json:"verified_at,omitempty"`

	// RejectedAt is set when the verification reaches a rejected state.
	RejectedAt *time.Time `db:"rejected_at" json:"rejected_at,omitempty"`

	// ExpiresAt indicates when this verification result becomes too old for
	// future BNPL decisions.
	ExpiresAt *time.Time `db:"expires_at" json:"expires_at,omitempty"`

	// Timestamps stores creation and update timestamps.
	Timestamps
}
