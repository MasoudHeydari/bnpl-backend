package v1

import (
	"sort"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	// PolicyID identifies the repayment priority policy family.
	PolicyID = "repayment_priority"

	// PolicyVersion identifies this repayment priority policy revision.
	PolicyVersion = "v1"
)

// Candidate is one per-contract next-payable installment considered for
// global repayment priority selection.
type Candidate struct {
	// Installment is the next payable installment in its contract sequence.
	Installment model.Installment

	// HasUnresolvedPayment is true when a non-terminal payment attempt exists
	// for this installment.
	HasUnresolvedPayment bool

	// HasOutstandingCharges permits repayment of residual charges after principal settlement.
	HasOutstandingCharges bool
}

// Selection is the outcome of evaluating global repayment priority.
type Selection struct {
	// Selected is the globally highest-priority payable installment when one
	// exists.
	Selected *model.Installment

	// HasUnresolvedPayment reports whether the selected installment currently
	// has a non-terminal payment attempt.
	HasUnresolvedPayment bool

	// PolicyVersion identifies this policy revision.
	PolicyVersion string
}

// Policy is the v1 repayment priority policy.
type Policy struct{}

// New returns the v1 repayment priority policy.
func New() Policy {
	return Policy{}
}

// Select chooses the customer's globally highest-priority payable installment.
//
// candidates must already be filtered to the next payable installment per
// active contract (per-contract sequencing). Among those:
//   - past-due / due / overdue obligations take precedence over scheduled ones
//   - earlier due_at wins
//   - installment ID breaks remaining ties deterministically
//
// An unresolved payment on the selected installment does not cause the policy
// to skip to a later installment; callers must surface the pending state.
func (Policy) Select(candidates []Candidate, now time.Time) Selection {
	now = now.UTC()
	out := Selection{PolicyVersion: PolicyVersion}
	if len(candidates) == 0 {
		return out
	}

	type ranked struct {
		candidate Candidate
		pastDue   bool
	}

	rankedCandidates := make([]ranked, 0, len(candidates))
	for _, candidate := range candidates {
		inst := candidate.Installment
		if inst.ContractID == nil || inst.ContractID.IsZero() {
			continue
		}

		if !inst.Status.IsActive() && !(inst.Status == model.PaidInstallmentStatus && candidate.HasOutstandingCharges) {
			continue
		}

		pastDue := !inst.DueAt.UTC().After(now) ||
			inst.Status == model.DueInstallmentStatus ||
			inst.Status == model.OverdueInstallmentStatus

		rankedCandidates = append(rankedCandidates, ranked{
			candidate: candidate,
			pastDue:   pastDue,
		})
	}

	if len(rankedCandidates) == 0 {
		return out
	}

	sort.SliceStable(rankedCandidates, func(i, j int) bool {
		a, b := rankedCandidates[i], rankedCandidates[j]
		if a.pastDue != b.pastDue {
			return a.pastDue
		}

		aDue := a.candidate.Installment.DueAt.UTC()
		bDue := b.candidate.Installment.DueAt.UTC()
		if !aDue.Equal(bDue) {
			return aDue.Before(bDue)
		}

		return a.candidate.Installment.ID.String() < b.candidate.Installment.ID.String()
	})

	selected := rankedCandidates[0].candidate.Installment
	out.Selected = &selected
	out.HasUnresolvedPayment = rankedCandidates[0].candidate.HasUnresolvedPayment

	return out
}

// IsSelected reports whether installmentID matches the selected priority target.
func (s Selection) IsSelected(installmentID model.InstallmentID) bool {
	return s.Selected != nil && s.Selected.ID == installmentID
}
