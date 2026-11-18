package v3

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/collections"
	legacyv1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"
	legacyv2 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v2"
	"github.com/stretchr/testify/require"
)

func TestV3_DueOverdueDefaultBoundaries(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)

	contract := model.ContractID(uuid.New())
	due := time.Date(2026, 11, 1, 18, 0, 0, 0, loc)
	grace := uint(4)
	i := model.Installment{
		ID:              model.InstallmentID(uuid.New()),
		ContractID:      &contract,
		Status:          model.ScheduledInstallmentStatus,
		DueAt:           due,
		GracePeriodDays: grace,
	}
	facts := []collections.ObligationFact{FactFromUnpaidInstallment(i, false)}
	p := New(loc)

	overdueAt := i.OverdueEligibleAt(loc)
	defaultAt := collections.DefaultEligibleAt(due, grace, DefaultAfterDays, loc)
	require.Equal(t, overdueAt, p.Boundary(i))
	require.Equal(t, defaultAt, p.DefaultBoundary(i))
	require.Equal(t, overdueAt.AddDate(0, 0, int(DefaultAfterDays)), defaultAt)

	// Before due: no block, no overdue, no default.
	beforeDue := due.Add(-time.Minute)
	eval := p.Evaluate(facts, beforeDue, false)
	require.False(t, eval.BlocksNewCredit)
	require.False(t, eval.IsOverdueEligible)
	require.False(t, eval.IsDefaultEligible)
	require.Empty(t, eval.UnsettledDueInstallmentIDs)

	// At due: blocks credit, not overdue/default.
	atDue := due.UTC()
	eval = p.Evaluate(facts, atDue, false)
	require.True(t, eval.BlocksNewCredit)
	require.False(t, eval.IsOverdueEligible)
	require.False(t, eval.IsDefaultEligible)
	require.Equal(t, []model.InstallmentID{i.ID}, eval.UnsettledDueInstallmentIDs)

	// During grace (day 2–5 relative to due day): due-block only.
	graceMid := time.Date(due.Year(), due.Month(), due.Day(), 12, 0, 0, 0, loc).AddDate(0, 0, 2)
	eval = p.Evaluate(facts, graceMid, false)
	require.True(t, eval.BlocksNewCredit)
	require.False(t, eval.IsOverdueEligible)
	require.False(t, eval.IsDefaultEligible)

	// At overdue boundary (day 6 / D+Grace+1 midnight): overdue, not default.
	eval = p.Evaluate(facts, overdueAt, false)
	require.True(t, eval.BlocksNewCredit)
	require.True(t, eval.IsOverdueEligible)
	require.False(t, eval.IsDefaultEligible)
	require.Equal(t, []model.InstallmentID{i.ID}, eval.OverdueInstallmentIDs)
	require.Empty(t, eval.TriggerInstallmentIDs)

	// Just before default: still overdue-only.
	eval = p.Evaluate(facts, defaultAt.Add(-time.Nanosecond), false)
	require.True(t, eval.IsOverdueEligible)
	require.False(t, eval.IsDefaultEligible)
	require.Equal(t, []model.InstallmentID{i.ID}, eval.OverdueInstallmentIDs)

	// At default boundary (+90 from overdue): default-eligible trigger.
	eval = p.Evaluate(facts, defaultAt, false)
	require.True(t, eval.BlocksNewCredit)
	require.True(t, eval.IsOverdueEligible)
	require.True(t, eval.IsDefaultEligible)
	require.Equal(t, []model.InstallmentID{i.ID}, eval.TriggerInstallmentIDs)
	require.Empty(t, eval.OverdueInstallmentIDs)
}

func TestV3_LateFeeAmountMinor(t *testing.T) {
	require.Equal(t, uint64(0), LateFeeAmountMinor(0))
	require.Equal(t, uint64(167), LateFeeAmountMinor(100000))
	require.Equal(t, uint64(1), LateFeeAmountMinor(600)) // 600*167/100000 = 1
	require.Equal(t, uint64(0), LateFeeAmountMinor(500)) // 500*167/100000 truncates to 0
}

func TestV3_RegistryKeepsHistorical(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)

	r, err := collections.NewRegistry(New(loc), legacyv1.New(), legacyv2.New(loc))
	require.NoError(t, err)
	require.Equal(t, "v3", r.Current.Version())

	v1, err := r.Resolve(PolicyID, "v1")
	require.NoError(t, err)
	require.Equal(t, "v1", v1.Version())

	v2, err := r.Resolve(PolicyID, "v2")
	require.NoError(t, err)
	require.Equal(t, "v2", v2.Version())
}
