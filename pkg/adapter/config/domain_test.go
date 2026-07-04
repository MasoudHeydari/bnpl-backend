package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestDomain_Validate_SupportedSubset(t *testing.T) {
	t.Parallel()

	require.NoError(t, Domain{
		SupportedJurisdictions: []model.Jurisdiction{model.IRJurisdiction},
	}.validate())
}

func TestDomain_Validate_RejectsEmptyDuplicatesAndUnsupported(t *testing.T) {
	t.Parallel()

	err := Domain{}.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not be empty")

	err = Domain{
		SupportedJurisdictions: []model.Jurisdiction{},
	}.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not be empty")

	err = Domain{
		SupportedJurisdictions: []model.Jurisdiction{
			model.IRJurisdiction,
			model.IRJurisdiction,
		},
	}.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate")

	err = Domain{
		SupportedJurisdictions: []model.Jurisdiction{model.Jurisdiction("TR")},
	}.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported by this BNPL build")

	err = Domain{
		SupportedJurisdictions: []model.Jurisdiction{
			model.IRJurisdiction,
			model.Jurisdiction("XX"),
		},
	}.validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "XX")
}

func TestConfig_NewJurisdictionCapabilities(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Domain: Domain{
			SupportedJurisdictions: []model.Jurisdiction{model.IRJurisdiction},
		},
	}

	caps, err := cfg.NewJurisdictionCapabilities()
	require.NoError(t, err)
	require.True(t, caps.IsOriginationEnabled(model.IRJurisdiction))
	require.NoError(t, caps.RequireOriginationEnabled(model.IRJurisdiction))
	require.ErrorIs(
		t,
		caps.RequireOriginationEnabled(model.Jurisdiction("TR")),
		domaincapability.ErrOriginationNotEnabled,
	)

	phone, err := cfg.NewPhoneCanonicalizer(caps)
	require.NoError(t, err)
	require.NotNil(t, phone)

	_, err = (&Config{Domain: Domain{}}).NewJurisdictionCapabilities()
	require.Error(t, err)
}
