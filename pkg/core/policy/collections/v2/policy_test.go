package v2

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/collections"
	legacy "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"
	"github.com/stretchr/testify/require"
)

func TestCalendarGraceAndVersionSelection(t *testing.T) {
	for _, zone := range []string{"Asia/Tehran", "America/New_York"} {
		location, err := time.LoadLocation(zone)
		require.NoError(t, err)
		for _, date := range []time.Time{time.Date(2026, 11, 1, 18, 0, 0, 0, location), time.Date(2026, 3, 7, 18, 0, 0, 0, location)} {
			for _, days := range []uint{0, 1, 5, 45} {
				contract := model.ContractID(uuid.New())
				i := model.Installment{ID: model.InstallmentID(uuid.New()), ContractID: &contract, Status: model.ScheduledInstallmentStatus, DueAt: date, GracePeriodDays: days}
				facts := []collections.ObligationFact{FactFromUnpaidInstallment(i, false)}
				boundary := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, location).AddDate(0, 0, int(days)+1)
				p := New(location)
				require.Equal(t, boundary.UTC(), p.Boundary(i))
				for day := 0; day <= int(days); day++ {
					at := time.Date(date.Year(), date.Month(), date.Day(), 23, 59, 59, 0, location).AddDate(0, 0, day)
					require.False(t, p.Evaluate(facts, at, false).IsDefaultEligible)
				}
				require.False(t, p.Evaluate(facts, boundary.Add(-time.Nanosecond), false).IsDefaultEligible)
				require.True(t, p.Evaluate(facts, boundary, false).IsDefaultEligible)
				r, err := collections.NewRegistry(p, legacy.New())
				require.NoError(t, err)
				old, err := r.Resolve("collections_default", "v1")
				require.NoError(t, err)
				require.Equal(t, date.UTC().AddDate(0, 0, int(days)), old.Boundary(i))
			}
		}
	}
}
