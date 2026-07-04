package initialcreditlimit

import (
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/policy"
)

// ErrPolicyNotFound indicates no policy is registered for a requested Ref.
// Callers can check for it with errors.Is.
var ErrPolicyNotFound = errors.New("policy not found")

// Registry resolves an initial credit-limit policy version by Ref. It is
// built once during bootstrap from explicitly compiled-in versions and is
// immutable and safe for concurrent read use afterward. There is no init()
// registration, no reflection, and no mutation after construction.
type Registry struct {
	policies map[policy.Ref]Policy
}

// NewRegistry builds an explicit registry from the supplied versions.
// Bootstrap is the only intended caller. A nil policy, a zero/incomplete
// ref, or a duplicate ref fails construction.
func NewRegistry(policies ...Policy) (*Registry, error) {
	const op = "initialcreditlimit.NewRegistry"

	set := make(map[policy.Ref]Policy, len(policies))

	for _, p := range policies {
		if p == nil {
			return nil, fmt.Errorf("%s: initial credit limit policy registry: policy must not be nil", op)
		}

		ref := p.Ref()
		if ref.ID == "" || ref.Version == "" {
			return nil, fmt.Errorf("%s: initial credit limit policy registry: ref must not be empty", op)
		}

		if _, exists := set[ref]; exists {
			return nil, fmt.Errorf("%s: initial credit limit policy registry: duplicate registration for ref %s", op, ref)
		}

		set[ref] = p
	}

	return &Registry{policies: set}, nil
}

// Resolve returns the policy registered for ref, or ErrPolicyNotFound if
// unregistered.
func (r *Registry) Resolve(ref policy.Ref) (Policy, error) {
	const op = "initialcreditlimit.Registry.Resolve"

	if r != nil {
		if p, ok := r.policies[ref]; ok {
			return p, nil
		}
	}

	return nil, fmt.Errorf("%s: initial credit limit policy registry: unknown ref %s: %w", op, ref, ErrPolicyNotFound)
}
