package merchantuc

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
)

// PaginationMode selects keyset or page-number pagination for list operations.
type PaginationMode string

// Supported merchant pagination modes.
const (
	// PaginationModeKeyset uses after_id / after_created_at seek pagination.
	PaginationModeKeyset PaginationMode = "keyset"

	// PaginationModePage uses page / limit offset pagination.
	PaginationModePage PaginationMode = "page"
)

// IsValid reports whether mode is a supported pagination mode.
func (m PaginationMode) IsValid() bool {
	return m == PaginationModeKeyset || m == PaginationModePage
}

// GetProfileInput identifies the authenticated external merchant (Identity Service subject).
//
// MerchantID is merchant_profiles.merchant_id from x-auth-subject-id, not
// merchant_profiles.id.
type GetProfileInput struct {
	// MerchantID is the Identity Service subject mapped to merchant_profiles.merchant_id.
	MerchantID model.MerchantID
}

// GetProfileOutput carries merchant display attributes and BNPL participation settings.
//
// RiskTier is intentionally omitted from this merchant-facing projection.
type GetProfileOutput struct {
	// MerchantID is the Identity Service subject mapped to merchant_profiles.merchant_id.
	MerchantID model.MerchantID
	// DisplayName is the merchant display name.
	DisplayName string
	// LegalName is the merchant legal name.
	LegalName string
	// LogoURL is the merchant logo URL.
	LogoURL string
	// SupportPhone is the merchant support phone number.
	SupportPhone string
	// SupportEmail is the merchant support email address.
	SupportEmail string
	// Status is the current lifecycle status.
	Status model.MerchantStatus
	// FundingModels lists supported funding models.
	FundingModels []model.FundingModel
	// SettlementModels lists supported settlement models.
	SettlementModels []model.SettlementModel
	// Currencies lists supported currencies.
	Currencies []model.Currency
	// MaxAmountMinor is the maximum allowed purchase amount in minor currency units.
	MaxAmountMinor uint64
	// MaxAmountCurrency is the currency for MaxAmountMinor.
	MaxAmountCurrency model.Currency
	// MaxTenureMonths is the maximum tenure in months.
	MaxTenureMonths uint
	// RequiresBranchMode indicates whether branch-scoped checkout is required.
	RequiresBranchMode bool
	// Jurisdiction is the operating jurisdiction.
	Jurisdiction model.Jurisdiction
	// CreatedAt is when the record was created.
	CreatedAt time.Time
	// UpdatedAt is when the record was last updated.
	UpdatedAt time.Time
}

// ListBranchesInput identifies the authenticated external merchant and pagination mode.
//
// MerchantID is the Identity Service subject ID (x-auth-subject-id /
// merchant_profiles.merchant_id), not merchant_profiles.id.
//
// Exactly one of Keyset or Offset is consumed according to Mode.
type ListBranchesInput struct {
	// MerchantID is the Identity Service subject mapped to merchant_profiles.merchant_id.
	MerchantID model.MerchantID
	// Mode selects the pagination mode.
	Mode PaginationMode
	// Keyset carries keyset pagination parameters or results.
	Keyset pagination.KeysetRequest
	// Offset carries offset pagination parameters or results.
	Offset pagination.OffsetRequest
}

// Branch is an API-facing merchant selling-point (branch profile) summary.
// A branch may represent either a physical location or an online storefront.
type Branch struct {
	// ID uniquely identifies the record.
	ID model.MerchantBranchProfileID
	// BranchID is the external branch identity for the branch profile.
	BranchID model.BranchID
	// Type identifies whether this selling point is physical or online.
	Type model.MerchantBranchType
	// Status is the current lifecycle status.
	Status model.BranchStatus
	// FundingModels lists supported funding models.
	FundingModels []model.FundingModel
	// SettlementModels lists supported settlement models.
	SettlementModels []model.SettlementModel
	// Currencies lists supported currencies.
	Currencies []model.Currency
	// RiskTierOverride overrides the merchant risk tier when set.
	RiskTierOverride *model.RiskTier
	// MaxAmountOverrideMinor overrides the merchant max amount in minor currency units when set.
	MaxAmountOverrideMinor *uint64
	// MaxAmountOverrideCurrency is the currency for MaxAmountOverrideMinor when set.
	MaxAmountOverrideCurrency *model.Currency
	// MaxTenureMonthsOverride overrides the merchant max tenure in months when set.
	MaxTenureMonthsOverride *uint
	// Jurisdiction is the operating jurisdiction.
	Jurisdiction model.Jurisdiction
	// CreatedAt is when the record was created.
	CreatedAt time.Time
	// UpdatedAt is when the record was last updated.
	UpdatedAt time.Time
}

// ListBranchesOutput carries the branch page and pagination metadata for the selected mode.
//
// Keyset is set when Mode is keyset. Page is set when Mode is page.
type ListBranchesOutput struct {
	// Branches lists the branch records.
	Branches []Branch
	// Keyset carries keyset pagination parameters or results.
	Keyset *pagination.KeysetMeta
	// Page carries pagination request or response metadata.
	Page *pagination.PageMeta
}

// ListSalesInput identifies the authenticated external merchant and pagination mode.
//
// MerchantID is the Identity Service subject ID (x-auth-subject-id /
// merchant_profiles.merchant_id), not merchant_profiles.id.
//
// Exactly one of Keyset or Offset is consumed according to Mode.
type ListSalesInput struct {
	// MerchantID is the Identity Service subject mapped to merchant_profiles.merchant_id.
	MerchantID model.MerchantID
	// Mode selects the pagination mode.
	Mode PaginationMode
	// Keyset carries keyset pagination parameters or results.
	Keyset pagination.KeysetRequest
	// Offset carries offset pagination parameters or results.
	Offset pagination.OffsetRequest
}

// Sale is an API-facing active merchant sale (active contract) summary.
type Sale struct {
	// ID uniquely identifies the record.
	ID model.ContractID
	// CustomerID is the internal customer_profiles.id primary key.
	CustomerID model.CustomerID
	// MerchantBranchProfileID identifies the merchant branch profile when present.
	MerchantBranchProfileID *model.MerchantBranchProfileID
	// RequestedAmountMinor is the requested amount in minor currency units.
	RequestedAmountMinor uint64
	// RemainingPrincipalAmountMinor is the remaining principal amount in minor currency units.
	RemainingPrincipalAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// InstallmentCount is the number of installments in the schedule.
	InstallmentCount int
	// Status is the current lifecycle status.
	Status model.ContractStatus
	// ActivatedAt is when the contract or sale became active.
	ActivatedAt *time.Time
	// CreatedAt is when the record was created.
	CreatedAt time.Time
}

// ListSalesOutput carries the sales page and pagination metadata for the selected mode.
//
// Keyset is set when Mode is keyset. Page is set when Mode is page.
type ListSalesOutput struct {
	// Sales lists the sale records.
	Sales []Sale
	// Keyset carries keyset pagination parameters or results.
	Keyset *pagination.KeysetMeta
	// Page carries pagination request or response metadata.
	Page *pagination.PageMeta
}
