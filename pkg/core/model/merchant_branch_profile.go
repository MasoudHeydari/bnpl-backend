package model

import (
	"time"

	"github.com/google/uuid"
)

// BranchStatus identifies the lifecycle state of a merchant branch's BNPL
// participation.
type BranchStatus string

const (
	// PendingReviewMerchantBranchStatus indicates the branch has been added but
	// is awaiting approval for BNPL participation.
	PendingReviewMerchantBranchStatus BranchStatus = "pending_review"

	// ActiveMerchantBranchStatus indicates the branch may offer BNPL to
	// customers.
	ActiveMerchantBranchStatus BranchStatus = "active"

	// DeactivatedMerchantBranchStatus indicates the branch voluntarily or
	// administratively stopped offering BNPL.
	DeactivatedMerchantBranchStatus BranchStatus = "deactivated"

	// SuspendedMerchantBranchStatus indicates the branch is temporarily
	// blocked from BNPL activity, typically pending review.
	SuspendedMerchantBranchStatus BranchStatus = "suspended"

	// TerminatedMerchantBranchStatus indicates the branch's BNPL
	// participation has ended.
	TerminatedMerchantBranchStatus BranchStatus = "terminated"
)

// MerchantBranchProfile represents a per-branch override of a merchant's
// BNPL configuration.
//
// It applies when the parent MerchantProfile.RequiresBranchMode is true,
// letting individual branches customize funding, settlement, currency, and
// exposure limits within the parent merchant's configuration. Pointer override
// fields use nil to fall back to the corresponding MerchantProfile setting.
type MerchantBranchProfile struct {
	// ID uniquely identifies this branch profile record.
	ID uuid.UUID `db:"id" json:"id,omitempty"`

	// MerchantID references the parent merchant's identity record.
	MerchantID uuid.UUID `db:"merchant_id" json:"merchant_id,omitempty"`

	// BranchID references the branch's identity record.
	BranchID uuid.UUID `db:"branch_id" json:"branch_id,omitempty"`

	// Status is the current lifecycle state of the branch's BNPL
	// participation.
	Status BranchStatus `db:"status" json:"status,omitempty"`

	// FundingModels lists the funding arrangements available to this
	// branch.
	FundingModels []FundingModel `db:"funding_models" json:"funding_models,omitempty"`

	// SettlementModels lists the settlement arrangements available to this
	// branch.
	SettlementModels []SettlementModel `db:"settlement_models" json:"settlement_models,omitempty"`

	// Currencies lists the currencies this branch may transact in.
	Currencies []Currency `db:"currencies" json:"currencies,omitempty"`

	// RiskTierOverride, when set, replaces the parent MerchantProfile's
	// RiskTier for this branch.
	RiskTierOverride *RiskTier `db:"risk_tier_override" json:"risk_tier_override,omitempty"`

	// MaxAmountOverride, when set, replaces the parent MerchantProfile's
	// MaxAmount for this branch.
	MaxAmountOverride *Money `db:"max_amount_override" json:"max_amount_override,omitempty"`

	// MaxTenureMonthsOverride, when set, replaces the parent
	// MerchantProfile's MaxTenureMonths for this branch.
	MaxTenureMonthsOverride *uint `db:"max_tenure_months_override" json:"max_tenure_months_override,omitempty"`

	// Jurisdiction is the regulatory jurisdiction governing this branch's
	// BNPL activity.
	Jurisdiction Jurisdiction `db:"jurisdiction" json:"jurisdiction,omitempty"`

	// CreatedAt is when the profile was created.
	CreatedAt time.Time `db:"created_at" json:"created_at,omitempty"`

	// UpdatedAt is when the profile was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at,omitempty"`
}
