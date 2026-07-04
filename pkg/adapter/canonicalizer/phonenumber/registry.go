package phonenumber

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/canonicalizer/phonenumber/iran"
	corephonenumber "github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// countryCanonicalizer is a local, side-effect-free per-jurisdiction adapter.
type countryCanonicalizer interface {
	Canonicalize(raw string) (string, error)
}

// Registry supplies compiled phone canonicalizers.
//
// Construction consumes domaincapability.Jurisdictions so every
// origination-enabled jurisdiction is proven to have a compiled adapter.
// Canonicalize remains available for every compiled adapter so legitimate
// replay/parsing is not broken merely because origination was later disabled.
type Registry struct {
	adapters map[model.Jurisdiction]countryCanonicalizer
}

var _ corephonenumber.Canonicalizer = (*Registry)(nil)

// compiledImplementations are adapters present in this binary. Origination
// capability selects which must be present at startup; compilation alone does
// not admit new business.
var compiledImplementations = map[model.Jurisdiction]func() countryCanonicalizer{
	model.IRJurisdiction: func() countryCanonicalizer { return iran.New() },
}

// NewRegistry constructs a Canonicalizer from the deployment origination
// capability. Every jurisdiction in jurisdictions.EnabledForOrigination() must
// have a compiled phone adapter or construction fails.
func NewRegistry(jurisdictions *domaincapability.Jurisdictions) (*Registry, error) {
	return newRegistry(jurisdictions, compiledImplementations)
}

func newRegistry(
	jurisdictions *domaincapability.Jurisdictions,
	factories map[model.Jurisdiction]func() countryCanonicalizer,
) (*Registry, error) {
	const op = "phonenumber.NewRegistry"

	if jurisdictions == nil {
		return nil, fmt.Errorf("%s: jurisdiction capability is required", op)
	}

	for _, jurisdiction := range jurisdictions.EnabledForOrigination() {
		if !jurisdiction.IsSupported() {
			return nil, fmt.Errorf(
				"%s: jurisdiction %q is not supported by this BNPL build",
				op,
				jurisdiction,
			)
		}

		if _, ok := factories[jurisdiction]; !ok {
			return nil, fmt.Errorf(
				"%s: jurisdiction %q has no compiled phone canonicalizer",
				op,
				jurisdiction,
			)
		}
	}

	if len(factories) == 0 {
		return nil, fmt.Errorf("%s: no compiled phone canonicalizers", op)
	}

	adapters := make(map[model.Jurisdiction]countryCanonicalizer, len(factories))
	for jurisdiction, factory := range factories {
		adapters[jurisdiction] = factory()
	}

	return &Registry{adapters: adapters}, nil
}

// Canonicalize validates and canonicalizes raw using the compiled adapter for
// jurisdiction. Country is never inferred from the phone prefix.
//
// Missing adapter availability returns ErrUnsupportedJurisdiction. Business
// origination admission is enforced separately by domaincapability.Jurisdictions.
func (r *Registry) Canonicalize(
	jurisdiction model.Jurisdiction,
	raw string,
) (string, error) {
	adapter, ok := r.adapters[jurisdiction]
	if !ok {
		return "", corephonenumber.ErrUnsupportedJurisdiction
	}

	return adapter.Canonicalize(raw)
}
