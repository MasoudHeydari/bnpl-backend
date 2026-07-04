package config

import (
	"fmt"

	adapterphonenumber "github.com/parstis/bnpl-backend/pkg/adapter/canonicalizer/phonenumber"
	corephonenumber "github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Domain configures deployment-enabled domain capabilities.
//
// model.SupportedJurisdictions() declares what this BNPL build can support.
// Domain.SupportedJurisdictions selects which of those jurisdictions this
// deployment currently allows for new-business/origination.
//
// Disabling a jurisdiction must prevent new origination; it must not by itself
// block servicing of already-created obligations.
type Domain struct {
	// SupportedJurisdictions are jurisdictions enabled for new-business
	// origination in this deployment. Every entry must be present in
	// model.SupportedJurisdictions().
	SupportedJurisdictions []model.Jurisdiction `yaml:"supported_jurisdictions"`
}

func (d Domain) validate() error {
	const op = "config.Domain.validate"

	if len(d.SupportedJurisdictions) == 0 {
		return fmt.Errorf("%s: domain.supported_jurisdictions must not be empty", op)
	}

	seen := make(map[model.Jurisdiction]struct{}, len(d.SupportedJurisdictions))
	for _, jurisdiction := range d.SupportedJurisdictions {
		if _, dup := seen[jurisdiction]; dup {
			return fmt.Errorf(
				"%s: domain.supported_jurisdictions contains duplicate %q",
				op,
				jurisdiction,
			)
		}

		seen[jurisdiction] = struct{}{}

		if !jurisdiction.IsSupported() {
			return fmt.Errorf(
				"%s: domain.supported_jurisdictions value %q is not supported by this BNPL build",
				op,
				jurisdiction,
			)
		}
	}

	return nil
}

// NewJurisdictionCapabilities constructs the immutable Core capability for
// origination-enabled jurisdictions from domain.supported_jurisdictions.
func (c *Config) NewJurisdictionCapabilities() (*domaincapability.Jurisdictions, error) {
	const op = "config.Config.NewJurisdictionCapabilities"

	caps, err := domaincapability.NewJurisdictions(c.Domain.SupportedJurisdictions)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return caps, nil
}

// NewPhoneCanonicalizer constructs the jurisdiction-aware phone canonicalizer
// from the already-created origination capability.
//
// Phone Registry is not the business-admission authority; it consumes
// jurisdictions to ensure every origination-enabled jurisdiction has a
// compiled adapter.
func (c *Config) NewPhoneCanonicalizer(
	jurisdictions *domaincapability.Jurisdictions,
) (corephonenumber.Canonicalizer, error) {
	const op = "config.Config.NewPhoneCanonicalizer"

	reg, err := adapterphonenumber.NewRegistry(jurisdictions)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return reg, nil
}
