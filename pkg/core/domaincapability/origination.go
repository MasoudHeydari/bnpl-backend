package domaincapability

import "github.com/parstis/bnpl-backend/pkg/core/model"

// OriginationJurisdictions is the business-facing view of which jurisdictions
// may originate new BNPL business in this deployment.
//
// Use cases depend on this narrow interface. Composition/adapter wiring that
// needs enumeration (for example Phone Registry) uses the concrete
// *Jurisdictions type.
type OriginationJurisdictions interface {
	IsOriginationEnabled(jurisdiction model.Jurisdiction) bool
	RequireOriginationEnabled(jurisdiction model.Jurisdiction) error
}

var _ OriginationJurisdictions = (*Jurisdictions)(nil)
