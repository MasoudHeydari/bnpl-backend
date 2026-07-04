package domaincapability

import (
	"errors"
	"fmt"
	"slices"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ErrOriginationNotEnabled reports that a jurisdiction may not originate new
// BNPL business in this deployment. It does not imply that existing obligations
// in that jurisdiction become unserviceable.
var ErrOriginationNotEnabled = errors.New("jurisdiction origination is not enabled for this deployment")

// Jurisdictions is the immutable set of jurisdictions for which this deployment
// may originate new BNPL business. It is constructed once at startup from
// domain.supported_jurisdictions and must not be mutated afterward.
//
// Semantics:
//
//	domain.supported_jurisdictions
//	=
//	jurisdictions in which this deployment may originate NEW BNPL business
//
// Disabling a jurisdiction must prevent new checkout/decision/origination
// admission. It must not block servicing of already-created obligations
// (repayment, reconciliation, contract lifecycle, and similar).
//
// Phone canonicalization is a separate concern under
// pkg/core/canonicalizer/phonenumber.
type Jurisdictions struct {
	enabled map[model.Jurisdiction]struct{}
	// ordered is a stable copy of the construction input for read-only views.
	ordered []model.Jurisdiction
}

// NewJurisdictions constructs an origination-enabled jurisdiction capability.
//
// enabled must be a non-empty, duplicate-free subset of
// model.SupportedJurisdictions(). Invalid input fails construction so the
// process cannot start with an inconsistent deployment capability.
func NewJurisdictions(enabled []model.Jurisdiction) (*Jurisdictions, error) {
	const op = "domaincapability.NewJurisdictions"

	if len(enabled) == 0 {
		return nil, fmt.Errorf("%s: at least one origination-enabled jurisdiction is required", op)
	}

	seen := make(map[model.Jurisdiction]struct{}, len(enabled))
	ordered := make([]model.Jurisdiction, 0, len(enabled))

	for _, jurisdiction := range enabled {
		if _, dup := seen[jurisdiction]; dup {
			return nil, fmt.Errorf("%s: duplicate origination-enabled jurisdiction %q", op, jurisdiction)
		}

		if !jurisdiction.IsSupported() {
			return nil, fmt.Errorf(
				"%s: jurisdiction %q is not supported by this BNPL build",
				op,
				jurisdiction,
			)
		}

		seen[jurisdiction] = struct{}{}
		ordered = append(ordered, jurisdiction)
	}

	return &Jurisdictions{enabled: seen, ordered: ordered}, nil
}

// IsOriginationEnabled reports whether jurisdiction may originate new BNPL
// business in this deployment.
func (j *Jurisdictions) IsOriginationEnabled(jurisdiction model.Jurisdiction) bool {
	if j == nil {
		return false
	}

	_, ok := j.enabled[jurisdiction]

	return ok
}

// RequireOriginationEnabled fails closed when jurisdiction may not originate
// new BNPL business in this deployment. The returned error identifies the
// jurisdiction for operators; callers must map it to an internal/domain
// failure rather than a customer validation error.
func (j *Jurisdictions) RequireOriginationEnabled(jurisdiction model.Jurisdiction) error {
	const op = "domaincapability.Jurisdictions.RequireOriginationEnabled"

	if j == nil {
		return fmt.Errorf("%s: jurisdiction capability is nil: %w", op, ErrOriginationNotEnabled)
	}

	if !j.IsOriginationEnabled(jurisdiction) {
		return fmt.Errorf("%s: jurisdiction %q: %w", op, jurisdiction, ErrOriginationNotEnabled)
	}

	return nil
}

// EnabledForOrigination returns a defensive copy of the jurisdictions enabled
// for new-business origination in this deployment.
//
// This method is for composition/adapter wiring (for example Phone Registry).
// Business use cases should depend on OriginationJurisdictions instead.
func (j *Jurisdictions) EnabledForOrigination() []model.Jurisdiction {
	if j == nil {
		return nil
	}

	return slices.Clone(j.ordered)
}
