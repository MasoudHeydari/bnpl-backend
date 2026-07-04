package catalog

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	checkoutv1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v1"
	checkoutv2 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v2"
)

// DefaultRef returns the checkout policy used when a subject has no explicit assignment.
func DefaultRef() policy.Ref { return checkoutv1.New().Ref() }

// TrustedRef returns the checkout policy assigned to trusted customers.
func TrustedRef() policy.Ref { return checkoutv2.New().Ref() }

// NewRegistry constructs the immutable compiled checkout policy registry.
func NewRegistry() (*checkout.Registry, error) {
	const op = "catalog.NewRegistry"

	registry, err := checkout.NewRegistry(checkoutv1.New(), checkoutv2.New())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return registry, nil
}
