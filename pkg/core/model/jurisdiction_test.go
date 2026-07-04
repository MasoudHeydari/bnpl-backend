package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSupportedJurisdictions_ContainsIR(t *testing.T) {
	t.Parallel()

	got := SupportedJurisdictions()
	require.Contains(t, got, IRJurisdiction)
	require.True(t, IRJurisdiction.IsSupported())
	require.True(t, IRJurisdiction.IsValid())
}

func TestSupportedJurisdictions_ReturnedSliceIsCopy(t *testing.T) {
	t.Parallel()

	got := SupportedJurisdictions()
	require.NotEmpty(t, got)

	got[0] = Jurisdiction("XX")
	require.True(t, IRJurisdiction.IsSupported())
	require.Contains(t, SupportedJurisdictions(), IRJurisdiction)
}

func TestJurisdiction_Unsupported(t *testing.T) {
	t.Parallel()

	require.False(t, Jurisdiction("TR").IsSupported())
	require.False(t, Jurisdiction("XX").IsSupported())
	require.False(t, Jurisdiction("").IsSupported())
}
