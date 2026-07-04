package catalog

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	initialcreditlimitv1 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v1"
	initialcreditlimitv2 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v2"
)

// DefaultRef returns the policy for customers without an explicit assignment.
func DefaultRef() policy.Ref { return initialcreditlimitv1.New().Ref() }

// TrustedRef returns the initial-credit policy assigned to trusted customers.
func TrustedRef() policy.Ref { return initialcreditlimitv2.New().Ref() }

// NewRegistry constructs the immutable compiled initial-credit policy registry.
func NewRegistry() (*initialcreditlimit.Registry, error) {
	const op = "catalog.NewRegistry"

	registry, err := initialcreditlimit.NewRegistry(initialcreditlimitv1.New(), initialcreditlimitv2.New())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return registry, nil
}
