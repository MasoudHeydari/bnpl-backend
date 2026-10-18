package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MerchantID is the external merchant identity (merchant_profiles.merchant_id).
type MerchantID uuid.UUID

// ParseMerchantID parses a MerchantID from its canonical string form.
func ParseMerchantID(value string) (MerchantID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return MerchantID{}, fmt.Errorf("parse merchant id: %w", err)
	}

	return MerchantID(id), nil
}

func (id MerchantID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id MerchantID) String() string { return uuid.UUID(id).String() }

func (id MerchantID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id MerchantID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *MerchantID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = MerchantID(u)

	return nil
}

func (id *MerchantID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = MerchantID(u)

	return nil
}

func (id MerchantID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// MerchantProfileID is the internal BNPL primary key for a merchant profile.
type MerchantProfileID uuid.UUID

// ParseMerchantProfileID parses a MerchantProfileID from its canonical string form.
func ParseMerchantProfileID(value string) (MerchantProfileID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return MerchantProfileID{}, fmt.Errorf("parse merchant profile id: %w", err)
	}

	return MerchantProfileID(id), nil
}

func (id MerchantProfileID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id MerchantProfileID) String() string { return uuid.UUID(id).String() }

func (id MerchantProfileID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id MerchantProfileID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *MerchantProfileID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = MerchantProfileID(u)

	return nil
}

func (id *MerchantProfileID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = MerchantProfileID(u)

	return nil
}

func (id MerchantProfileID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

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

var merchantStatusTransitions = newStatusTransitionTable(
	"merchant status",
	map[MerchantStatus][]MerchantStatus{
		PendingReviewMerchantStatus: {
			ActiveMerchantStatus,
			DeactivatedMerchantStatus,
			SuspendedMerchantStatus,
			TerminatedMerchantStatus,
		},
		ActiveMerchantStatus: {
			DeactivatedMerchantStatus,
			SuspendedMerchantStatus,
			TerminatedMerchantStatus,
		},
		DeactivatedMerchantStatus: {
			ActiveMerchantStatus,
			SuspendedMerchantStatus,
			TerminatedMerchantStatus,
		},
		SuspendedMerchantStatus: {
			ActiveMerchantStatus,
			DeactivatedMerchantStatus,
			TerminatedMerchantStatus,
		},
	},
	[]MerchantStatus{
		TerminatedMerchantStatus,
	},
)

// IsValid reports whether the merchant status is known by the domain.
func (s MerchantStatus) IsValid() bool {
	return merchantStatusTransitions.isValid(s)
}

// CanTransitionTo reports whether the merchant status may change to next.
func (s MerchantStatus) CanTransitionTo(next MerchantStatus) bool {
	return merchantStatusTransitions.canTransitionTo(s, next)
}

// ValidateTransitionTo returns an error when the merchant status may not change to next.
func (s MerchantStatus) ValidateTransitionTo(next MerchantStatus) error {
	return merchantStatusTransitions.validateTransitionTo(s, next)
}

// IsTerminal reports whether the merchant status is terminal.
func (s MerchantStatus) IsTerminal() bool {
	return merchantStatusTransitions.isTerminal(s)
}

// IsActive reports whether the merchant may offer BNPL.
func (s MerchantStatus) IsActive() bool {
	return s == ActiveMerchantStatus
}

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
// currencies. MerchantID is the external subject identity (today stored here;
// later owned by Merchant Service). Display fields (name, logo, contacts) are
// temporarily denormalized in BNPL until that service exists. When
// RequiresBranchMode is true, individual branches may override these settings
// via MerchantBranchProfile.
type MerchantProfile struct {
	// ID uniquely identifies this BNPL profile record.
	ID MerchantProfileID `db:"id" json:"id,omitempty"`

	// MerchantID references the merchant's identity record.
	MerchantID MerchantID `db:"merchant_id" json:"merchant_id,omitempty"`

	// DisplayName is the merchant's public/trade name for UI surfaces.
	DisplayName string `db:"display_name" json:"display_name,omitempty"`

	// LegalName is the registered legal entity name, when known.
	LegalName string `db:"legal_name" json:"legal_name,omitempty"`

	// LogoURL is an optional absolute URL for the merchant logo.
	LogoURL string `db:"logo_url" json:"logo_url,omitempty"`

	// SupportPhone is an optional customer-support phone number.
	SupportPhone string `db:"support_phone" json:"support_phone,omitempty"`

	// SupportEmail is an optional customer-support email address.
	SupportEmail string `db:"support_email" json:"support_email,omitempty"`

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

	// MaxAmountMinor is the maximum BNPL transaction amount permitted for this
	// merchant.
	MaxAmountMinor uint64 `db:"max_amount_minor" json:"max_amount_minor,omitempty"`

	// MaxAmountCurrency identifies the currency of MaxAmountMinor.
	MaxAmountCurrency Currency `db:"max_amount_currency" json:"max_amount_currency,omitempty"`

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
