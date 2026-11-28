package merchantprovisioninguc

// These are BNPL-owned initial merchant financing terms, not environment
// settings. The initial maximum follows the lower of the two existing Dev
// merchant fixtures; it is not an approved production underwriting policy.
// A future merchant-specific policy can replace these onboarding defaults.
const (
	defaultMerchantMaxAmountMinor  uint64 = 1_000_000_000
	defaultMerchantMaxTenureMonths uint   = 4
)
