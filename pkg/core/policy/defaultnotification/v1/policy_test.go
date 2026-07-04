package v1

import (
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/stretchr/testify/require"
)

func TestDefaultNoticeWindowAndTruth(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)
	due := time.Date(2026, 11, 1, 18, 0, 0, 0, loc)
	for _, days := range []uint{0, 5, 45} {
		boundary := (model.Installment{DueAt: due, GracePeriodDays: days}).OverdueEligibleAt(loc)
		for _, offset := range []time.Duration{-time.Second, 0, 12*time.Hour + 59*time.Minute, 13 * time.Hour, 13*time.Hour + 5*time.Minute, 3*24*time.Hour + 13*time.Hour} {
			now := boundary.Add(offset)
			require.Equal(t, offset >= 13*time.Hour, Eligible(now, boundary, loc, true, true, false))
			require.False(t, Eligible(now, boundary, loc, false, true, false))
			require.False(t, Eligible(now, boundary, loc, true, false, false))
			require.False(t, Eligible(now, boundary, loc, true, true, true))
		}
	}
}
