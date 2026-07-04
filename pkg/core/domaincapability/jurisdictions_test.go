package domaincapability

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestNewJurisdictions_EnablesIRForOrigination(t *testing.T) {
	t.Parallel()

	caps, err := NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction})
	require.NoError(t, err)
	require.True(t, caps.IsOriginationEnabled(model.IRJurisdiction))
	require.NoError(t, caps.RequireOriginationEnabled(model.IRJurisdiction))
	require.False(t, caps.IsOriginationEnabled(model.Jurisdiction("TR")))
	require.ErrorIs(t, caps.RequireOriginationEnabled(model.Jurisdiction("TR")), ErrOriginationNotEnabled)
}

func TestNewJurisdictions_RejectsEmptyDuplicateAndUnsupported(t *testing.T) {
	t.Parallel()

	_, err := NewJurisdictions(nil)
	require.Error(t, err)

	_, err = NewJurisdictions([]model.Jurisdiction{})
	require.Error(t, err)

	_, err = NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction, model.IRJurisdiction})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")

	_, err = NewJurisdictions([]model.Jurisdiction{model.Jurisdiction("TR")})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported")
}

func TestJurisdictions_NilReceiverFailsClosed(t *testing.T) {
	t.Parallel()

	var caps *Jurisdictions
	require.False(t, caps.IsOriginationEnabled(model.IRJurisdiction))
	require.ErrorIs(t, caps.RequireOriginationEnabled(model.IRJurisdiction), ErrOriginationNotEnabled)
	require.Nil(t, caps.EnabledForOrigination())
}

func TestJurisdictions_ImplementsOriginationJurisdictions(t *testing.T) {
	t.Parallel()

	caps, err := NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction})
	require.NoError(t, err)

	var _ OriginationJurisdictions = caps
}

func TestJurisdictions_EnabledForOriginationDefensiveCopy(t *testing.T) {
	t.Parallel()

	caps, err := NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction})
	require.NoError(t, err)

	got := caps.EnabledForOrigination()
	require.Equal(t, []model.Jurisdiction{model.IRJurisdiction}, got)

	got[0] = model.Jurisdiction("TR")
	require.Equal(t, []model.Jurisdiction{model.IRJurisdiction}, caps.EnabledForOrigination())
	require.True(t, caps.IsOriginationEnabled(model.IRJurisdiction))
	require.False(t, caps.IsOriginationEnabled(model.Jurisdiction("TR")))
}
