package model

import (
	"time"

	"github.com/google/uuid"
)

// CreditAllocationStrategy determines how a limit is assessed, independently
// from the source of eligibility. Risk-based assessment is reserved until a
// real scoring algorithm and provider are registered.
type CreditAllocationStrategy string

const (
	DisabledCreditAllocationStrategy   CreditAllocationStrategy = "disabled"
	FixedLimitCreditAllocationStrategy CreditAllocationStrategy = "fixed_limit"
	RiskBasedCreditAllocationStrategy  CreditAllocationStrategy = "risk_based"
	DenyCreditAllocationStrategy       CreditAllocationStrategy = "deny"
	InheritCreditAllocationStrategy    CreditAllocationStrategy = "inherit"
)

type CreditAllocationScope string

const (
	DefaultCreditAllocationScope    CreditAllocationScope = "default"
	GroupCreditAllocationScope      CreditAllocationScope = "group"
	IndividualCreditAllocationScope CreditAllocationScope = "individual"
)

// CreditGroup is a named, administratively managed eligibility group. Group
// changes affect subsequent grants, not already assigned credit capacity.
type CreditGroup struct {
	Code      string    `db:"code" json:"code"`
	Name      string    `db:"name" json:"name"`
	Enabled   bool      `db:"enabled" json:"enabled"`
	UpdatedBy string    `db:"updated_by" json:"updated_by"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// CreditEligibilityEntry stores a pre-registration identity pair. The owner
// link may only be established after authoritative CRS verification.
type CreditEligibilityEntry struct {
	ID            uuid.UUID      `db:"id" json:"id"`
	NationalCode  string         `db:"national_code" json:"national_code"`
	CreditOwnerID *CreditOwnerID `db:"credit_owner_id" json:"credit_owner_id,omitempty"`
	CreatedBy     string         `db:"created_by" json:"created_by"`
	CreatedAt     time.Time      `db:"created_at" json:"created_at"`
}

// CreditEligibilityMobile is an immutable imported alias of a registry entry.
// Its existence is not evidence of identity ownership.
type CreditEligibilityMobile struct {
	MobileE164    string    `db:"mobile_e164" json:"mobile_e164"`
	EligibilityID uuid.UUID `db:"eligibility_id" json:"eligibility_id"`
	CreatedBy     string    `db:"created_by" json:"created_by"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

// CreditGroupMembership is an append-only revision; a nil group terminates
// membership. There is at most one effective group per eligibility entry.
type CreditGroupMembership struct {
	EligibilityID uuid.UUID `db:"eligibility_id" json:"eligibility_id"`
	Version       int64     `db:"version" json:"version"`
	GroupCode     *string   `db:"group_code" json:"group_code"`
	CreatedBy     string    `db:"created_by" json:"created_by"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

// CreditAllocationRule is an immutable configuration revision. A completed
// request references this exact row, not whichever rule happens to be current.
type CreditAllocationRule struct {
	ID               uuid.UUID                `db:"id" json:"id"`
	Scope            CreditAllocationScope    `db:"scope" json:"scope"`
	GroupCode        *string                  `db:"group_code" json:"group_code,omitempty"`
	EligibilityID    *uuid.UUID               `db:"eligibility_id" json:"eligibility_id,omitempty"`
	CreditKind       CreditKind               `db:"credit_kind" json:"credit_kind"`
	Currency         Currency                 `db:"currency" json:"currency"`
	Strategy         CreditAllocationStrategy `db:"strategy" json:"strategy"`
	FixedAmountMinor uint64                   `db:"fixed_amount_minor" json:"fixed_amount_minor"`
	Version          int64                    `db:"version" json:"version"`
	CreatedBy        string                   `db:"created_by" json:"created_by"`
	CreatedAt        time.Time                `db:"created_at" json:"created_at"`
}
