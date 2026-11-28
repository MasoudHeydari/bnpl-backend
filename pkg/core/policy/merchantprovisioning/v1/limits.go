package v1

// Platform ceilings apply to merchant declarations in every deployment.
// IRR has no fractional minor units: 5 billion rials equal 500 million tomans.
const (
	platformMaxAmountMinor  uint64 = 5_000_000_000
	platformMaxTenureMonths uint   = 12
)
