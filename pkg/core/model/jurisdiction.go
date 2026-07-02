package model

// Jurisdiction identifies the legal or regulatory scope that governs a BNPL profile.
type Jurisdiction string

const (
	// IRJurisdiction represents Iran.
	IRJurisdiction Jurisdiction = "IR"
)

// IsValid reports whether the jurisdiction is known by the domain.
func (j Jurisdiction) IsValid() bool {
	switch j {
	case IRJurisdiction:
		return true
	default:
		return false
	}
}
