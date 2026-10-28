package collections

import (
	"fmt"
	"sort"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ObligationFact carries the explicit facts needed to evaluate one installment
// for collections default entry and recovery.
type ObligationFact struct {
	HasUnresolvedAdjustment   bool
	HasUnresolvedCancellation bool
	// InstallmentID identifies the installment.
	InstallmentID model.InstallmentID

	// ContractID identifies the owning active contract when present.
	ContractID *model.ContractID

	// DueAt is the contractual due instant.
	DueAt time.Time

	// GracePeriodDays is the persisted contractual grace snapshot.
	GracePeriodDays uint

	// Status is the current installment lifecycle status.
	Status model.InstallmentStatus

	// IsFullySettled is true only when Financial Core-authoritative settlement
	// has fully satisfied the obligation.
	IsFullySettled bool

	// HasUnresolvedPayment is true when a non-terminal payment attempt exists.
	HasUnresolvedPayment bool

	// IsContractuallyApplicable excludes cancelled/expired/inapplicable rows.
	IsContractuallyApplicable bool
}

// Evaluation is the pure collections policy outcome for one customer.
type Evaluation struct {
	HasUnresolvedAdjustment bool
	// HasUnresolvedCancellation blocks new credit without claiming debt settlement.
	HasUnresolvedCancellation bool
	// IsDefaultEligible is true when at least one applicable outstanding
	// installment has crossed its grace boundary.
	IsDefaultEligible bool

	// IsRecovered is true when the customer is currently in default and every
	// installment whose due date has arrived is fully settled.
	IsRecovered bool

	// TriggerInstallmentIDs lists installments that justify default entry.
	TriggerInstallmentIDs []model.InstallmentID

	// UnsettledDueInstallmentIDs lists due obligations that block recovery.
	UnsettledDueInstallmentIDs []model.InstallmentID

	// HasPaymentUncertainty is true when an unsettled due obligation has a
	// non-terminal payment attempt.
	HasPaymentUncertainty bool

	// PolicyID identifies the policy family.
	PolicyID string

	// PolicyVersion identifies this policy revision.
	PolicyVersion string
}

// Policy is a deterministic, versioned Collections rule set.
type Policy interface {
	ID() string
	Version() string
	Evaluate([]ObligationFact, time.Time, bool) Evaluation
	Boundary(model.Installment) time.Time
}

// Registry selects current entry rules and preserves opening-policy recovery.
type Registry struct {
	Current  Policy
	versions map[string]Policy
}

func NewRegistry(current Policy, historical ...Policy) (*Registry, error) {
	if current == nil {
		return nil, fmt.Errorf("collections current policy is required")
	}
	r := &Registry{Current: current, versions: map[string]Policy{}}
	for _, p := range append(historical, current) {
		if p == nil {
			return nil, fmt.Errorf("nil collections policy")
		}
		key := p.ID() + ":" + p.Version()
		if _, ok := r.versions[key]; ok {
			return nil, fmt.Errorf("duplicate collections policy %s", key)
		}
		r.versions[key] = p
	}
	return r, nil
}
func (r *Registry) Resolve(id, version string) (Policy, error) {
	p, ok := r.versions[id+":"+version]
	if !ok {
		return nil, fmt.Errorf("unknown collections policy %s:%s", id, version)
	}
	return p, nil
}

// Evaluate applies shared debt/recovery predicates using the selected version
// only for its contractual boundary. It is specific to Collections.
func Evaluate(p Policy, facts []ObligationFact, now time.Time, currentlyDefaulted bool) Evaluation {
	now = now.UTC()
	out := Evaluation{
		PolicyID:      p.ID(),
		PolicyVersion: p.Version(),
	}

	triggers := make([]model.InstallmentID, 0)
	unsettledDue := make([]model.InstallmentID, 0)

	for _, fact := range facts {
		if !fact.IsContractuallyApplicable {
			continue
		}

		if fact.ContractID == nil || fact.ContractID.IsZero() {
			continue
		}

		if !fact.Status.IsActive() && !fact.IsFullySettled {
			// Non-active, unsettled rows are ignored for collections unless
			// already settled; cancelled/expired are not contractual debt.
			continue
		}

		if fact.HasUnresolvedCancellation {
			out.HasUnresolvedCancellation = true
		}
		dueAt := fact.DueAt.UTC()
		overdueEligibleAt := p.Boundary(model.Installment{DueAt: dueAt, GracePeriodDays: fact.GracePeriodDays})

		if fact.HasUnresolvedAdjustment {
			out.HasUnresolvedAdjustment = true
		}
		if !fact.HasUnresolvedAdjustment && !fact.IsFullySettled && !now.Before(overdueEligibleAt) {
			triggers = append(triggers, fact.InstallmentID)
			if fact.HasUnresolvedPayment || fact.HasUnresolvedCancellation || fact.HasUnresolvedAdjustment {
				out.HasPaymentUncertainty = true
			}
		}

		if !dueAt.After(now) && !fact.IsFullySettled {
			unsettledDue = append(unsettledDue, fact.InstallmentID)
			if fact.HasUnresolvedPayment || fact.HasUnresolvedCancellation || fact.HasUnresolvedAdjustment {
				out.HasPaymentUncertainty = true
			}
		}
	}

	sort.Slice(triggers, func(i, j int) bool {
		return triggers[i].String() < triggers[j].String()
	})
	sort.Slice(unsettledDue, func(i, j int) bool {
		return unsettledDue[i].String() < unsettledDue[j].String()
	})

	out.TriggerInstallmentIDs = triggers
	out.UnsettledDueInstallmentIDs = unsettledDue
	out.IsDefaultEligible = len(triggers) > 0

	if currentlyDefaulted {
		out.IsRecovered = len(unsettledDue) == 0
	}

	return out
}

// FactFromUnpaidInstallment maps persisted debt without infrastructure coupling.
func FactFromUnpaidInstallment(i model.Installment, unresolved bool) ObligationFact {
	return ObligationFact{InstallmentID: i.ID, ContractID: i.ContractID,
		DueAt: i.DueAt, GracePeriodDays: i.GracePeriodDays, Status: i.Status,
		HasUnresolvedPayment: unresolved, IsContractuallyApplicable: true}
}
