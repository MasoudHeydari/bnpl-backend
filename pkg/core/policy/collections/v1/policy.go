package v1

import (
	"sort"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	// PolicyID identifies the collections default policy family.
	PolicyID = "collections_default"

	// PolicyVersion identifies this collections policy revision.
	PolicyVersion = "v1"
)

// ObligationFact carries the explicit facts needed to evaluate one installment
// for collections default entry and recovery.
type ObligationFact struct {
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

// Policy is the v1 collections default/recovery policy.
type Policy struct{}

// New returns the v1 collections policy.
func New() Policy {
	return Policy{}
}

// Evaluate assesses default entry and recovery predicates for one customer.
//
// Default entry: at least one applicable, unsettled installment with
// now >= OverdueEligibleAt (due_at + grace_period_days * 24h), matching
// model.Installment.OverdueEligibleAt / SQL grace eligibility.
//
// Recovery (only meaningful when currentlyDefaulted): every applicable
// installment whose due_at <= now must be fully settled. Future unpaid
// installments do not block recovery. Unresolved payments are not settlement.
func (Policy) Evaluate(
	facts []ObligationFact,
	now time.Time,
	currentlyDefaulted bool,
) Evaluation {
	now = now.UTC()
	out := Evaluation{
		PolicyID:      PolicyID,
		PolicyVersion: PolicyVersion,
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

		dueAt := fact.DueAt.UTC()
		overdueEligibleAt := overdueEligibleAt(dueAt, fact.GracePeriodDays)

		if !fact.IsFullySettled && !now.Before(overdueEligibleAt) {
			triggers = append(triggers, fact.InstallmentID)
			if fact.HasUnresolvedPayment {
				out.HasPaymentUncertainty = true
			}
		}

		if !dueAt.After(now) && !fact.IsFullySettled {
			unsettledDue = append(unsettledDue, fact.InstallmentID)
			if fact.HasUnresolvedPayment {
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

func overdueEligibleAt(dueAt time.Time, gracePeriodDays uint) time.Time {
	return (model.Installment{DueAt: dueAt, GracePeriodDays: gracePeriodDays}).OverdueEligibleAt()
}

// FactFromUnpaidInstallment maps a contractually applicable outstanding row to
// the same policy input for assessments and live credit eligibility checks.
func FactFromUnpaidInstallment(i model.Installment, unresolved bool) ObligationFact {
	return ObligationFact{InstallmentID: i.ID, ContractID: i.ContractID,
		DueAt: i.DueAt, GracePeriodDays: i.GracePeriodDays, Status: i.Status,
		HasUnresolvedPayment: unresolved, IsContractuallyApplicable: true}
}
