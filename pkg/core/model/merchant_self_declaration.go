package model

// MerchantSelfDeclaration contains merchant-owned requested financing limits.
// These are neither deployment configuration nor platform-assigned defaults.
type MerchantSelfDeclaration struct {
	MaxAmountMinor    uint64
	MaxAmountCurrency Currency
	MaxTenureMonths   uint
}
