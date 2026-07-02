package model

import (
	"time"

	"github.com/google/uuid"
)

// BorrowerCreditLimitStatus identifies the lifecycle state of a borrower credit
// limit assignment.
type BorrowerCreditLimitStatus string

const (
	// ActiveBorrowerCreditLimitStatus indicates the credit limit is currently
	// valid and may be used for BNPL exposure checks.
	ActiveBorrowerCreditLimitStatus BorrowerCreditLimitStatus = "active"

	// ExpiredBorrowerCreditLimitStatus indicates the credit limit is no longer
	// valid because its validity window ended.
	ExpiredBorrowerCreditLimitStatus BorrowerCreditLimitStatus = "expired"

	// RevokedBorrowerCreditLimitStatus indicates the credit limit was explicitly
	// revoked by a later governed decision.
	RevokedBorrowerCreditLimitStatus BorrowerCreditLimitStatus = "revoked"

	// ReplacedBorrowerCreditLimitStatus indicates the credit limit was superseded
	// by a newer governed credit limit assignment.
	ReplacedBorrowerCreditLimitStatus BorrowerCreditLimitStatus = "replaced"
)

// BorrowerCreditLimit represents a governed BNPL credit limit assigned to a
// borrower.
//
// It stores the actual approved limit amount at the time of the credit limit
// decision. DecisionID links this limit to the decision audit envelope that
// explains why the limit was assigned, reduced, denied, or revoked.
type BorrowerCreditLimit struct {
	// ID uniquely identifies this credit limit assignment.
	ID uuid.UUID `db:"id" json:"id"`

	// BorrowerProfileID references the BNPL borrower profile this limit applies to.
	BorrowerProfileID uuid.UUID `db:"borrower_profile_id" json:"borrower_profile_id"`

	// DecisionID references the governed credit-limit Decision that authorized
	// this credit limit assignment.
	DecisionID uuid.UUID `db:"decision_id" json:"decision_id"`

	// Money stores the approved credit limit amount and currency.
	Money

	// Status is the current lifecycle state of this credit limit assignment.
	Status BorrowerCreditLimitStatus `db:"status" json:"status,omitempty"`

	// ValidityWindow defines when this credit limit is valid.
	ValidityWindow

	// RevokedAt indicates when this credit limit was explicitly revoked by a
	// later governed decision.
	RevokedAt *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`

	// ReplacedByCreditLimitID references the newer credit limit assignment that
	// replaced this one, when applicable.
	ReplacedByCreditLimitID *uuid.UUID `db:"replaced_by_credit_limit_id" json:"replaced_by_credit_limit_id,omitempty"`

	// Timestamps stores creation and update timestamps.
	Timestamps
}
