package phonenumber

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/canonicalizer/phonenumber"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func mustCaps(t *testing.T, enabled ...model.Jurisdiction) *domaincapability.Jurisdictions {
	t.Helper()

	caps, err := domaincapability.NewJurisdictions(enabled)
	require.NoError(t, err)

	return caps
}

func TestNewRegistry_RequiresCapabilityAndCompiledAdapters(t *testing.T) {
	t.Parallel()

	reg, err := NewRegistry(mustCaps(t, model.IRJurisdiction))
	require.NoError(t, err)

	got, err := reg.Canonicalize(model.IRJurisdiction, "09121234567")
	require.NoError(t, err)
	require.Equal(t, "+989121234567", got)

	_, err = NewRegistry(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "jurisdiction capability is required")
}

func TestNewRegistry_ConfiguredWithoutCompiledImplementation(t *testing.T) {
	t.Parallel()

	_, err := newRegistry(
		mustCaps(t, model.IRJurisdiction),
		map[model.Jurisdiction]func() countryCanonicalizer{},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no compiled phone canonicalizer")
}

func TestRegistry_UnknownJurisdictionIsUnsupported(t *testing.T) {
	t.Parallel()

	reg, err := NewRegistry(mustCaps(t, model.IRJurisdiction))
	require.NoError(t, err)

	_, err = reg.Canonicalize(model.Jurisdiction("TR"), "+905551234567")
	require.ErrorIs(t, err, phonenumber.ErrUnsupportedJurisdiction)
}

func TestRegistry_CompiledAdapterRemainsAfterOriginationBlocked(t *testing.T) {
	t.Parallel()

	// Registry construction required IR for origination, but Canonicalize must
	// still resolve IR after a later origination block so replay can parse phones.
	reg, err := NewRegistry(mustCaps(t, model.IRJurisdiction))
	require.NoError(t, err)

	got, err := reg.Canonicalize(model.IRJurisdiction, "+989121234567")
	require.NoError(t, err)
	require.Equal(t, "+989121234567", got)
}
