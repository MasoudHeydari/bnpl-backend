package v2

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/collections"
)

const (
	// PolicyID identifies the collections default policy family.
	PolicyID = "collections_default"

	// PolicyVersion identifies this collections policy revision.
	PolicyVersion = "v2"
)

// ObligationFact and Evaluation are shared domain facts for versioned Collections rules.
type ObligationFact = collections.ObligationFact
type Evaluation = collections.Evaluation

// Policy is the v2 collections default/recovery policy.
type Policy struct{ location *time.Location }

// New returns the v2 collections policy.
func New(location *time.Location) Policy {
	if location == nil {
		panic("collections business location is required")
	}
	return Policy{location: location}
}

// Evaluate assesses default entry and recovery predicates for one customer.
//
// Default entry: at least one applicable, unsettled installment with
// 00:00 on D+Grace+1 in the injected business location.
//
// Recovery (only meaningful when currentlyDefaulted): every applicable
// installment whose due_at <= now must be fully settled. Future unpaid
// installments do not block recovery. Unresolved payments are not settlement.
func (p Policy) Evaluate(facts []ObligationFact, now time.Time, currentlyDefaulted bool) Evaluation {
	return collections.Evaluate(p, facts, now, currentlyDefaulted)
}

// FactFromUnpaidInstallment maps a contractually applicable outstanding row to
// the same policy input for assessments and live credit eligibility checks.
func FactFromUnpaidInstallment(i model.Installment, unresolved bool) ObligationFact {
	return collections.FactFromUnpaidInstallment(i, unresolved)
}

func (Policy) ID() string      { return PolicyID }
func (Policy) Version() string { return PolicyVersion }

// Boundary applies calendar-day v2 contractual grace.
func (p Policy) Boundary(i model.Installment) time.Time { return i.OverdueEligibleAt(p.location) }
