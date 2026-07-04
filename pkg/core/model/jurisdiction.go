package model

// Jurisdiction identifies the legal or regulatory scope that governs a BNPL profile.
//
// MerchantProfile.Jurisdiction also selects customer-phone validation and
// canonicalization rules. A deployment enables a subset of the jurisdictions
// this binary can support for new-business/origination via
// domain.supported_jurisdictions.
type Jurisdiction string

const (
	// IRJurisdiction represents Iran.
	IRJurisdiction Jurisdiction = "IR"
)

// supportedJurisdictions is the immutable-in-practice set of jurisdictions this
// BNPL build is capable of supporting. Runtime config may enable only a subset.
// Add a new entry only when the corresponding phone (and related) adapters exist.
var supportedJurisdictions = [...]Jurisdiction{
	IRJurisdiction,
}

// SupportedJurisdictions returns a copy of the jurisdictions this binary can
// support. Callers may mutate the returned slice without affecting the set.
func SupportedJurisdictions() []Jurisdiction {
	out := make([]Jurisdiction, len(supportedJurisdictions))
	copy(out, supportedJurisdictions[:])

	return out
}

// IsSupported reports whether this jurisdiction is in the code-supported set.
func (j Jurisdiction) IsSupported() bool {
	for _, supported := range supportedJurisdictions {
		if j == supported {
			return true
		}
	}

	return false
}

// IsValid reports whether the jurisdiction is known by the domain.
// It matches IsSupported for the current model.
func (j Jurisdiction) IsValid() bool {
	return j.IsSupported()
}
