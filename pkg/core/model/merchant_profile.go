package model

import (
	"time"

	"github.com/google/uuid"
)

// MerchantStatus identifies the lifecycle state of a merchant's BNPL
// participation.
type MerchantStatus string

const (
	// PendingReviewMerchantStatus indicates the merchant has applied for
	// BNPL participation and is awaiting approval.
	PendingReviewMerchantStatus MerchantStatus = "pending_review"

	// ActiveMerchantStatus indicates the merchant may offer BNPL to
	// customers.
	ActiveMerchantStatus MerchantStatus = "active"

	// DeactivatedMerchantStatus indicates the merchant voluntarily or
	// administratively stopped offering BNPL.
	DeactivatedMerchantStatus MerchantStatus = "deactivated"

	// SuspendedMerchantStatus indicates the merchant is temporarily blocked
	// from BNPL activity, typically pending review.
	SuspendedMerchantStatus MerchantStatus = "suspended"

	// TerminatedMerchantStatus indicates the merchant relationship has
	// ended.
	TerminatedMerchantStatus MerchantStatus = "terminated"
)

// FundingModel identifies who provides the capital disbursed for a BNPL
// transaction.
type FundingModel string

const (
	// BankDirectFundingModel indicates a partner bank funds the transaction
	// directly.
	BankDirectFundingModel FundingModel = "bank_direct"

	// MerchantFundedFundingModel indicates the merchant funds the
	// transaction and is reimbursed through settlement.
	MerchantFundedFundingModel FundingModel = "merchant_funded"

	// PlatformFundedFundingModel indicates the BNPL platform funds the
	// transaction from its own balance sheet.
	PlatformFundedFundingModel FundingModel = "platform_funded"
)

// SettlementModel identifies how a merchant is paid for BNPL transactions.
type SettlementModel string

const (
	// DirectSettlementModel indicates the merchant is settled directly by
	// the platform.
	DirectSettlementModel SettlementModel = "direct"

	// BankSettlementModel indicates settlement is routed through a partner
	// bank.
	BankSettlementModel SettlementModel = "bank"

	// MerchantFundedSettlementModel indicates settlement reconciles against
	// funds the merchant already advanced.
	MerchantFundedSettlementModel SettlementModel = "merchant_funded"
)

// RiskTier classifies the credit risk associated with a merchant or branch,
// used to bound BNPL exposure limits.
type RiskTier string

const (
	// LowRiskTier indicates minimal credit risk.
	LowRiskTier RiskTier = "low"

	// MediumRiskTier indicates moderate credit risk.
	MediumRiskTier RiskTier = "medium"

	// HighRiskTier indicates elevated credit risk requiring tighter limits.
	HighRiskTier RiskTier = "high"
)

// MerchantProfile represents a merchant's BNPL participation configuration.
//
// It defines whether and how a merchant offers BNPL to customers, including
// funding and settlement arrangements, exposure limits, and supported
// currencies. MerchantID references the merchant's identity record, which is
// managed outside this package. When RequiresBranchMode is true, individual
// branches may override these settings via MerchantBranchProfile.
type MerchantProfile struct {
	// ID uniquely identifies this BNPL profile record.
	ID uuid.UUID `db:"id" json:"id,omitempty"`

	// MerchantID references the merchant's identity record.
	MerchantID uuid.UUID `db:"merchant_id" json:"merchant_id,omitempty"`

	// Status is the current lifecycle state of the merchant's BNPL
	// participation.
	Status MerchantStatus `db:"status" json:"status,omitempty"`

	// FundingModels lists the funding arrangements available to this
	// merchant.
	FundingModels []FundingModel `db:"funding_models" json:"funding_models,omitempty"`

	// SettlementModels lists the settlement arrangements available to this
	// merchant.
	SettlementModels []SettlementModel `db:"settlement_models" json:"settlement_models,omitempty"`

	// Currencies lists the currencies this merchant may transact in.
	Currencies []Currency `db:"currencies" json:"currencies,omitempty"`

	// RiskTier is the credit risk classification used to bound BNPL exposure
	// for this merchant.
	RiskTier RiskTier `db:"risk_tier" json:"risk_tier,omitempty"`

	// MaxAmount is the maximum BNPL transaction amount permitted for this
	// merchant.
	MaxAmount Money `db:"max_amount" json:"max_amount,omitempty"`

	// MaxTenureMonths is the maximum installment tenure, in months,
	// permitted for this merchant.
	MaxTenureMonths uint `db:"max_tenure_months" json:"max_tenure_months,omitempty"`

	// RequiresBranchMode indicates whether this merchant must configure
	// per-branch profiles instead of relying solely on this merchant-wide
	// configuration.
	RequiresBranchMode bool `db:"requires_branch_mode" json:"requires_branch_mode"`

	// Jurisdiction is the regulatory jurisdiction governing this merchant's
	// BNPL activity.
	Jurisdiction Jurisdiction `db:"jurisdiction" json:"jurisdiction,omitempty"`

	// CreatedAt is when the profile was created.
	CreatedAt time.Time `db:"created_at" json:"created_at,omitempty"`

	// UpdatedAt is when the profile was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at,omitempty"`
}
