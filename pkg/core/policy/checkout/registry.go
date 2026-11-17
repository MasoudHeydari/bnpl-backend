package checkout

import (
	"errors"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/policy"
)

// ErrPolicyNotFound indicates no policy is registered for a requested Ref.
// Callers can check for it with errors.Is.
var ErrPolicyNotFound = errors.New("policy not found")

// Registry resolves a checkout policy version by Ref. It is built once
// during bootstrap from explicitly compiled-in versions and is immutable and
// safe for concurrent read use afterward. There is no init() registration, no
// reflection, and no mutation after construction.
type Registry struct {
	policies map[policy.Ref]Policy
}

// NewRegistry builds an explicit registry from the supplied versions.
// Bootstrap is the only intended caller. A nil policy, a zero/incomplete
// ref, a duplicate ref, or invalid repayment/checkout terms fails construction
// so the process cannot start with an unsound catalog.
func NewRegistry(policies ...Policy) (*Registry, error) {
	const op = "checkout.NewRegistry"

	set := make(map[policy.Ref]Policy, len(policies))

	for _, p := range policies {
		if p == nil {
			return nil, fmt.Errorf("%s: checkout policy registry: policy must not be nil", op)
		}

		ref := p.Ref()
		if ref.ID == "" || ref.Version == "" {
			return nil, fmt.Errorf("%s: checkout policy registry: ref must not be empty", op)
		}

		if _, exists := set[ref]; exists {
			return nil, fmt.Errorf("%s: checkout policy registry: duplicate registration for ref %s", op, ref)
		}

		if err := ValidateTerms(p.Terms()); err != nil {
			return nil, fmt.Errorf("%s: checkout policy registry: invalid terms for ref %s: %w", op, ref, err)
		}

		set[ref] = p
	}

	return &Registry{policies: set}, nil
}

// Resolve returns the policy registered for ref, or ErrPolicyNotFound if
// unregistered.
func (r *Registry) Resolve(ref policy.Ref) (Policy, error) {
	const op = "checkout.Registry.Resolve"

	if r != nil {
		if p, ok := r.policies[ref]; ok {
			return p, nil
		}
	}

	return nil, fmt.Errorf("%s: checkout policy registry: unknown ref %s: %w", op, ref, ErrPolicyNotFound)
}

// MaxReminderLeadDays returns the largest Reminder.LeadDays among registered
// policies. Reminder candidate discovery uses this as the exclusive upper-bound
// horizon so every registered policy version remains discoverable.
func (r *Registry) MaxReminderLeadDays() (uint, error) {
	const op = "checkout.Registry.MaxReminderLeadDays"

	if r == nil || len(r.policies) == 0 {
		return 0, fmt.Errorf("%s: registry is empty", op)
	}

	var maxLead uint
	for ref, p := range r.policies {
		lead := p.Terms().Repayment.Reminder.LeadDays
		if lead == 0 {
			return 0, fmt.Errorf("%s: policy %s has zero reminder lead days", op, ref)
		}

		if lead > maxLead {
			maxLead = lead
		}
	}

	return maxLead, nil
}

// All returns a snapshot of registered policies for tests and diagnostics.
func (r *Registry) All() []Policy {
	if r == nil {
		return nil
	}

	out := make([]Policy, 0, len(r.policies))
	for _, p := range r.policies {
		out = append(out, p)
	}

	return out
}
