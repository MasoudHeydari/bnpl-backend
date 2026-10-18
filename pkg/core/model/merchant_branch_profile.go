package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BranchID is the external merchant branch identity.
type BranchID uuid.UUID

// ParseBranchID parses a BranchID from its canonical string form.
func ParseBranchID(value string) (BranchID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return BranchID{}, fmt.Errorf("parse branch id: %w", err)
	}

	return BranchID(id), nil
}

func (id BranchID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id BranchID) String() string { return uuid.UUID(id).String() }

func (id BranchID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id BranchID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *BranchID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = BranchID(u)

	return nil
}

func (id *BranchID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = BranchID(u)

	return nil
}

func (id BranchID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// MerchantBranchProfileID is the internal BNPL primary key for a branch profile.
type MerchantBranchProfileID uuid.UUID

// ParseMerchantBranchProfileID parses a MerchantBranchProfileID from its canonical string form.
func ParseMerchantBranchProfileID(value string) (MerchantBranchProfileID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return MerchantBranchProfileID{}, fmt.Errorf("parse merchant branch profile id: %w", err)
	}

	return MerchantBranchProfileID(id), nil
}

func (id MerchantBranchProfileID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id MerchantBranchProfileID) String() string { return uuid.UUID(id).String() }

func (id MerchantBranchProfileID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id MerchantBranchProfileID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

func (id *MerchantBranchProfileID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = MerchantBranchProfileID(u)

	return nil
}

func (id *MerchantBranchProfileID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = MerchantBranchProfileID(u)

	return nil
}

func (id MerchantBranchProfileID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// PosGpsID is the external POS/GPS device identity used at checkout.
type PosGpsID uuid.UUID

// ParsePosGpsID parses a PosGpsID from its canonical string form.
func ParsePosGpsID(value string) (PosGpsID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return PosGpsID{}, fmt.Errorf("parse pos gps id: %w", err)
	}

	return PosGpsID(id), nil
}

func (id PosGpsID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id PosGpsID) String() string { return uuid.UUID(id).String() }

func (id PosGpsID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id PosGpsID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *PosGpsID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = PosGpsID(u)

	return nil
}

func (id *PosGpsID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = PosGpsID(u)

	return nil
}

func (id PosGpsID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// OnlineStoreID is the external identity of an online merchant storefront.
type OnlineStoreID uuid.UUID

// ParseOnlineStoreID parses an OnlineStoreID from its canonical UUID string form.
func ParseOnlineStoreID(value string) (OnlineStoreID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return OnlineStoreID{}, fmt.Errorf("parse online store id: %w", err)
	}

	return OnlineStoreID(id), nil
}

func (id OnlineStoreID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id OnlineStoreID) String() string { return uuid.UUID(id).String() }

func (id OnlineStoreID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id OnlineStoreID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *OnlineStoreID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = OnlineStoreID(u)

	return nil
}

func (id *OnlineStoreID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = OnlineStoreID(u)

	return nil
}

func (id OnlineStoreID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// MerchantBranchType identifies the sales-surface kind represented by a
// MerchantBranchProfile. A branch profile is a merchant selling point and does
// not have to represent a physical location.
type MerchantBranchType string

const (
	// PhysicalMerchantBranchType represents a physical selling point that may own one or more POS terminals.
	PhysicalMerchantBranchType MerchantBranchType = "physical"

	// OnlineMerchantBranchType represents a virtual selling point addressed through an online store.
	OnlineMerchantBranchType MerchantBranchType = "online"
)

// IsValid reports whether the merchant branch type is supported by the domain.
func (t MerchantBranchType) IsValid() bool {
	return t == PhysicalMerchantBranchType || t == OnlineMerchantBranchType
}

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

var branchStatusTransitions = newStatusTransitionTable(
	"merchant branch status",
	map[BranchStatus][]BranchStatus{
		PendingReviewMerchantBranchStatus: {
			ActiveMerchantBranchStatus,
			DeactivatedMerchantBranchStatus,
			SuspendedMerchantBranchStatus,
			TerminatedMerchantBranchStatus,
		},
		ActiveMerchantBranchStatus: {
			DeactivatedMerchantBranchStatus,
			SuspendedMerchantBranchStatus,
			TerminatedMerchantBranchStatus,
		},
		DeactivatedMerchantBranchStatus: {
			ActiveMerchantBranchStatus,
			SuspendedMerchantBranchStatus,
			TerminatedMerchantBranchStatus,
		},
		SuspendedMerchantBranchStatus: {
			ActiveMerchantBranchStatus,
			DeactivatedMerchantBranchStatus,
			TerminatedMerchantBranchStatus,
		},
	},
	[]BranchStatus{
		TerminatedMerchantBranchStatus,
	},
)

// IsValid reports whether the branch status is known by the domain.
func (s BranchStatus) IsValid() bool {
	return branchStatusTransitions.isValid(s)
}

// CanTransitionTo reports whether the branch status may change to next.
func (s BranchStatus) CanTransitionTo(next BranchStatus) bool {
	return branchStatusTransitions.canTransitionTo(s, next)
}

// ValidateTransitionTo returns an error when the branch status may not change to next.
func (s BranchStatus) ValidateTransitionTo(next BranchStatus) error {
	return branchStatusTransitions.validateTransitionTo(s, next)
}

// IsTerminal reports whether the branch status is terminal.
func (s BranchStatus) IsTerminal() bool {
	return branchStatusTransitions.isTerminal(s)
}

// IsActive reports whether the branch may offer BNPL.
func (s BranchStatus) IsActive() bool {
	return s == ActiveMerchantBranchStatus
}

// MerchantBranchProfile represents a merchant selling point with optional
// per-selling-point overrides of the parent merchant BNPL configuration.
//
// A selling point may be physical or online. Physical points are addressed at
// checkout through merchant_pos_terminals, while online points are addressed
// through merchant_online_stores. Pointer override fields use nil to fall back
// to the corresponding MerchantProfile setting.
type MerchantBranchProfile struct {
	// ID uniquely identifies this branch profile record.
	ID MerchantBranchProfileID `db:"id" json:"id,omitempty"`

	// MerchantID references the parent merchant's identity record.
	MerchantID MerchantID `db:"merchant_id" json:"merchant_id,omitempty"`

	// BranchID references the branch's identity record.
	BranchID BranchID `db:"branch_id" json:"branch_id,omitempty"`

	// Type identifies whether this selling point is physical or online.
	Type MerchantBranchType `db:"branch_type" json:"branch_type,omitempty"`

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

	// MaxAmountOverrideMinor, when set, replaces the parent MerchantProfile's
	// MaxAmountMinor for this branch.
	MaxAmountOverrideMinor *uint64 `db:"max_amount_override_minor" json:"max_amount_override_minor,omitempty"`

	// MaxAmountOverrideCurrency, when set, replaces the parent MerchantProfile's
	// MaxAmountCurrency for this branch.
	MaxAmountOverrideCurrency *Currency `db:"max_amount_override_currency" json:"max_amount_override_currency,omitempty"`

	// MaxTenureMonthsOverride, when set, replaces the parent
	// MerchantProfile's MaxTenureMonths for this branch.
	MaxTenureMonthsOverride *uint `db:"max_tenure_months_override" json:"max_tenure_months_override,omitempty"`

	// Jurisdiction is the regulatory jurisdiction governing this branch's
	// BNPL activity.
	Jurisdiction Jurisdiction `db:"jurisdiction" json:"jurisdiction,omitempty"`

	// OfferPreparationMode controls when Partner Create prepares the financing
	// offer for checkouts at this online/physical selling point.
	//
	// Trusted provisioning sets this value. Merchants cannot select it through
	// Checkout Create input or terminal credentials. CheckoutSession snapshots
	// the effective mode at create time.
	OfferPreparationMode OfferPreparationMode `db:"offer_preparation_mode" json:"-"`

	// CreatedAt is when the profile was created.
	CreatedAt time.Time `db:"created_at" json:"created_at,omitempty"`

	// UpdatedAt is when the profile was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at,omitempty"`
}

// EffectiveOfferPreparationMode returns the branch preparation mode, defaulting
// to on_start when unset or invalid.
func (p MerchantBranchProfile) EffectiveOfferPreparationMode() OfferPreparationMode {
	if p.OfferPreparationMode.IsValid() {
		return p.OfferPreparationMode
	}

	return DefaultOfferPreparationMode
}
