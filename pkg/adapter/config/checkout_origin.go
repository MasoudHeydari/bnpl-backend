package config

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/checkouturl"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutorigin "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutorigin"
)

// PublicOriginPolicyConfig is the YAML-facing host allowlist for validating
// checkout_session.public_origin. Scheme/loopback/IP rules come from the
// runtime environment profile, not from this block.
//
// AllowedHosts must be configured independently of CHECKOUT_SESSION_PUBLIC_ORIGIN
// (do not derive the allowlist from the origin ENV value under validation).
type PublicOriginPolicyConfig struct {
	// AllowedHosts is the exact hostname allowlist (no ports). Production must
	// list the real public hostname(s) as literals in deployment config.
	AllowedHosts []string `yaml:"allowed_hosts"`
}

// publicOriginPolicyForEnvironment maps runtime environment onto a
// checkoutorigin.Policy. allowedHosts come from configuration and are never
// derived from CHECKOUT_SESSION_PUBLIC_ORIGIN itself.
func publicOriginPolicyForEnvironment(
	env model.RuntimeEnvironment,
	allowedHosts []string,
) checkoutorigin.Policy {
	switch env {
	case model.DevelopmentRuntimeEnvironment, model.TestRuntimeEnvironment:
		return checkoutorigin.Development(allowedHosts)

	default:
		return checkoutorigin.Strict(allowedHosts)
	}
}

// resolvePublicOrigin validates checkout_session.public_origin under the
// environment policy and returns a URL builder bound to the validated origin.
func (c CheckoutSession) resolvePublicOrigin(
	env model.RuntimeEnvironment,
) (checkoutorigin.PublicOrigin, checkouturl.Builder, error) {
	const op = "config.CheckoutSession.resolvePublicOrigin"

	policy := publicOriginPolicyForEnvironment(env, c.PublicOriginPolicy.AllowedHosts)

	origin, err := policy.Parse(c.PublicOrigin)
	if err != nil {
		return checkoutorigin.PublicOrigin{}, checkouturl.Builder{}, fmt.Errorf(
			"%s: %w",
			op,
			err,
		)
	}

	return origin, checkouturl.New(origin), nil
}
