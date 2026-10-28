package v1

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/policy/collections"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	// PolicyID identifies the collections default policy family.
	PolicyID = "collections_default"

	// PolicyVersion identifies this collections policy revision.
	PolicyVersion = "v1"
)

// ObligationFact and Evaluation retain source compatibility for v1 callers.
type ObligationFact = collections.ObligationFact
type Evaluation = collections.Evaluation

// Policy is the v1 collections default/recovery policy.
type Policy struct{}

// New returns the v1 collections policy.
func New() Policy {
	return Policy{}
}

// Evaluate assesses default entry and recovery predicates for one customer.
//
// Default entry: at least one applicable, unsettled installment with
// now >= due_at + grace_period_days elapsed UTC days. This frozen historical
// version deliberately differs from the calendar-day v2 policy used for new
// episodes, installment lifecycle transitions and current SQL selectors.
//
// Recovery (only meaningful when currentlyDefaulted): every applicable
// installment whose due_at <= now must be fully settled. Future unpaid
// installments do not block recovery. Unresolved payments are not settlement.
func (p Policy) Evaluate(facts []ObligationFact, now time.Time, currentlyDefaulted bool) Evaluation {
	return collections.Evaluate(p, facts, now, currentlyDefaulted)
}

func overdueEligibleAt(dueAt time.Time, gracePeriodDays uint) time.Time {
	return dueAt.UTC().AddDate(0, 0, int(gracePeriodDays))
}

// FactFromUnpaidInstallment maps a contractually applicable outstanding row to
// the same policy input for assessments and live credit eligibility checks.
func FactFromUnpaidInstallment(i model.Installment, unresolved bool) ObligationFact {
	return collections.FactFromUnpaidInstallment(i, unresolved)
}

func (Policy) ID() string      { return PolicyID }
func (Policy) Version() string { return PolicyVersion }

// Boundary preserves elapsed-day v1 evidence and never reinterprets history.
func (Policy) Boundary(i model.Installment) time.Time {
	return overdueEligibleAt(i.DueAt, i.GracePeriodDays)
}
