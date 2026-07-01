package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrInvalidBorrowerProfileID is returned when the borrower profile ID is empty.
	ErrInvalidBorrowerProfileID = errors.New("invalid borrower profile id")

	// ErrInvalidBorrowerID is returned when the referenced borrower ID is empty.
	ErrInvalidBorrowerID = errors.New("invalid borrower id")

	// ErrInvalidBorrowerStatus is returned when an unknown borrower status is used.
	ErrInvalidBorrowerStatus = errors.New("invalid borrower status")

	// ErrInvalidBorrowerJurisdiction is returned when jurisdiction is empty or invalid.
	ErrInvalidBorrowerJurisdiction = errors.New("invalid borrower jurisdiction")

	// ErrBorrowerDecisionRequired is returned when a governed state change has no Decision reference.
	ErrBorrowerDecisionRequired = errors.New("borrower decision required")

	// ErrTerminatedBorrowerProfile is returned when a terminal borrower profile is changed.
	ErrTerminatedBorrowerProfile = errors.New("terminated borrower profile cannot be changed")
)

// BorrowerStatus identifies the lifecycle state of a borrower's BNPL profile.
//
// This status describes the profile lifecycle only. Risk, fraud, compliance,
// or collections restrictions must be modeled separately as governed
// restrictions backed by Decisions.
type BorrowerStatus string

const (
	// PendingReviewBorrowerStatus indicates the BNPL profile exists but is not
	// yet approved for BNPL usage.
	PendingReviewBorrowerStatus BorrowerStatus = "pending_review"

	// ActiveBorrowerStatus indicates the borrower profile is active at the BNPL
	// profile level. Transaction-level eligibility still requires a Decision.
	ActiveBorrowerStatus BorrowerStatus = "active"

	// DeactivatedBorrowerStatus indicates the borrower profile was voluntarily
	// or administratively deactivated and cannot start new BNPL flows.
	DeactivatedBorrowerStatus BorrowerStatus = "deactivated"

	// TerminatedBorrowerStatus indicates the borrower relationship has ended
	// and the profile is retained only for audit and record-keeping.
	TerminatedBorrowerStatus BorrowerStatus = "terminated"
)

// IsValid reports whether the borrower status is known by the domain.
func (s BorrowerStatus) IsValid() bool {
	switch s {
	case PendingReviewBorrowerStatus,
		ActiveBorrowerStatus,
		DeactivatedBorrowerStatus,
		TerminatedBorrowerStatus:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the status is terminal.
func (s BorrowerStatus) IsTerminal() bool {
	return s == TerminatedBorrowerStatus
}

// BorrowerProfile represents a customer's BNPL-specific profile.
//
// It does not own raw identity data such as national ID, birthdate, or phone
// number. Those fields belong to the identity/KYC boundary. This profile only
// references the borrower identity and the latest verification/decision
// artifacts relevant to BNPL participation.
type BorrowerProfile struct {
	// ID uniquely identifies this BNPL profile record.
	ID uuid.UUID `db:"id" json:"id"`

	// BorrowerID references the borrower's identity record, which is owned by
	// the identity boundary.
	BorrowerID uuid.UUID `db:"borrower_id" json:"borrower_id"`

	// IdentityVerificationID references the identity verification artifact used
	// for this BNPL profile.
	IdentityVerificationID *uuid.UUID `db:"identity_verification_id" json:"identity_verification_id,omitempty"`

	// LastProfileEligibilityDecisionID references the latest profile-level
	// eligibility decision affecting this BNPL profile.
	LastProfileEligibilityDecisionID *uuid.UUID `db:"last_profile_eligibility_decision_id" json:"last_profile_eligibility_decision_id,omitempty"`

	// LastCreditLimitID references the latest credit limit assignment for this
	// borrower profile.
	LastCreditLimitID *uuid.UUID `db:"last_credit_limit_id" json:"last_credit_limit_id,omitempty"`

	// Jurisdiction identifies the regulatory jurisdiction that governs this
	// borrower's BNPL profile.
	Jurisdiction Jurisdiction `db:"jurisdiction" json:"jurisdiction,omitempty"`

	// Status is the current lifecycle state of the BNPL profile.
	Status BorrowerStatus `db:"status" json:"status,omitempty"`

	// CreatedAt is when the profile was created.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the profile was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// NewBorrowerProfile creates a new borrower BNPL profile in pending review state.
func NewBorrowerProfile(
	id uuid.UUID,
	borrowerID uuid.UUID,
	jurisdiction Jurisdiction,
	identityVerificationID *uuid.UUID,
	now time.Time,
) (BorrowerProfile, error) {
	if id == uuid.Nil {
		return BorrowerProfile{}, ErrInvalidBorrowerProfileID
	}

	if borrowerID == uuid.Nil {
		return BorrowerProfile{}, ErrInvalidBorrowerID
	}

	if !jurisdiction.IsValid() {
		return BorrowerProfile{}, ErrInvalidBorrowerJurisdiction
	}

	return BorrowerProfile{
		ID:                     id,
		BorrowerID:             borrowerID,
		IdentityVerificationID: identityVerificationID,
		Jurisdiction:           jurisdiction,
		Status:                 PendingReviewBorrowerStatus,
		CreatedAt:              now,
		UpdatedAt:              now,
	}, nil
}

// Validate verifies that the borrower profile is internally consistent.
func (p *BorrowerProfile) Validate() error {
	if p.ID == uuid.Nil {
		return ErrInvalidBorrowerProfileID
	}

	if p.BorrowerID == uuid.Nil {
		return ErrInvalidBorrowerID
	}

	if !p.Status.IsValid() {
		return ErrInvalidBorrowerStatus
	}

	if !p.Jurisdiction.IsValid() {
		return ErrInvalidBorrowerJurisdiction
	}

	return nil
}

// IsActive reports whether the borrower profile lifecycle is active.
//
// This does not mean the borrower is eligible for every BNPL transaction.
// Transaction approval must still be decided by eligibility and limit checks.
func (p *BorrowerProfile) IsActive() bool {
	return p.Status == ActiveBorrowerStatus
}

// Activate marks the borrower profile as active.
//
// Activation is a governed state change and must reference a Decision.
func (p *BorrowerProfile) Activate(decisionID uuid.UUID, now time.Time) error {
	if decisionID == uuid.Nil {
		return ErrBorrowerDecisionRequired
	}

	if p.Status.IsTerminal() {
		return ErrTerminatedBorrowerProfile
	}

	p.Status = ActiveBorrowerStatus
	p.LastProfileEligibilityDecisionID = &decisionID
	p.UpdatedAt = now

	return nil
}

// Deactivate marks the borrower profile as deactivated.
//
// Deactivation is reversible by a later governed activation.
func (p *BorrowerProfile) Deactivate(now time.Time) error {
	if p.Status.IsTerminal() {
		return ErrTerminatedBorrowerProfile
	}

	p.Status = DeactivatedBorrowerStatus
	p.UpdatedAt = now

	return nil
}

// Terminate marks the borrower profile as terminal.
//
// Terminated profiles are retained for audit and record-keeping and must not be
// reactivated.
func (p *BorrowerProfile) Terminate(now time.Time) error {
	p.Status = TerminatedBorrowerStatus
	p.UpdatedAt = now

	return nil
}
