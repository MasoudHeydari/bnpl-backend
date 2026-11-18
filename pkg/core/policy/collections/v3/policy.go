package v3

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/collections"
)

const (
	// PolicyID identifies the collections default policy family.
	PolicyID = "collections_default"

	// PolicyVersion identifies this collections policy revision.
	PolicyVersion = "v3"

	// DefaultAfterDays is the calendar-day lag from overdue eligibility to
	// default entry.
	DefaultAfterDays uint = 90

	// LateFeeRateNumerator is the non-compounding daily late-fee numerator.
	// Amount = principal * LateFeeRateNumerator / LateFeeRateDenominator.
	LateFeeRateNumerator uint64 = 167

	// LateFeeRateDenominator is the non-compounding daily late-fee denominator.
	LateFeeRateDenominator uint64 = 100000

	// BlockCreditOnDue reports that new credit is blocked once any installment
	// is contractually due and unsettled.
	BlockCreditOnDue = true
)

// ObligationFact and Evaluation are shared domain facts for versioned Collections rules.
type ObligationFact = collections.ObligationFact
type Evaluation = collections.Evaluation

// Policy is the v3 collections default/recovery policy.
type Policy struct{ location *time.Location }

// New returns the v3 collections policy.
func New(location *time.Location) Policy {
	if location == nil {
		panic("collections business location is required")
	}

	return Policy{location: location}
}

// Evaluate assesses due-block, overdue, default entry, and recovery predicates.
//
// Due block: unsettled installment with now >= due_at.
// Overdue: midnight on D+Grace+1 in the injected business location.
// Default entry: overdue boundary + DefaultAfterDays calendar days.
// Recovery (only when currentlyDefaulted): every installment with due_at <= now
// must be fully settled. Future unpaid installments do not block recovery.
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

// Boundary applies calendar-day contractual grace (overdue eligibility).
func (p Policy) Boundary(i model.Installment) time.Time {
	return i.OverdueEligibleAt(p.location)
}

// DefaultBoundary is overdue eligibility plus DefaultAfterDays calendar days.
func (p Policy) DefaultBoundary(i model.Installment) time.Time {
	return collections.DefaultEligibleAt(i.DueAt, i.GracePeriodDays, DefaultAfterDays, p.location)
}

// LateFeeAmountMinor returns the non-compounding late fee for one overdue
// business day from principal only.
func LateFeeAmountMinor(principalMinor uint64) uint64 {
	return principalMinor * LateFeeRateNumerator / LateFeeRateDenominator
}
