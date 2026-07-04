package customercredituc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAvailableCreditForInquiry_UsesOnlyActiveExposure(t *testing.T) {
	t.Parallel()

	available, used, err := availableCreditForInquiry(100_000, 25_000)
	require.NoError(t, err)
	require.Equal(t, uint64(25_000), used)
	require.Equal(t, uint64(75_000), available)
}

func TestAvailableCreditForInquiry_ZeroAvailableWhenOverLimit(t *testing.T) {
	t.Parallel()

	available, used, err := availableCreditForInquiry(100_000, 120_000)
	require.NoError(t, err)
	require.Equal(t, uint64(120_000), used)
	require.Equal(t, uint64(0), available)
}
