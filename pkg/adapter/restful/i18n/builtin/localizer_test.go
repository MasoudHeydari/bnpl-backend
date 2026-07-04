package builtin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
)

func TestLocalizer_LocalizeENUSAndFAIR(t *testing.T) {
	t.Parallel()

	localizer, err := New(resti18n.LocaleENUS)
	require.NoError(t, err)

	enUS, err := localizer.Localize(resti18n.LocaleENUS, "error.identity_subject_id_required", nil)
	require.NoError(t, err)
	assert.Equal(t, "Identity subject id is required.", enUS)

	faIR, err := localizer.Localize(resti18n.LocaleFAIR, "error.identity_subject_id_required", nil)
	require.NoError(t, err)
	assert.Equal(t, "شناسه هویت الزامی است.", faIR)
}

func TestLocalizer_RendersParams(t *testing.T) {
	t.Parallel()

	localizer, err := New(resti18n.LocaleENUS)
	require.NoError(t, err)

	message, err := localizer.Localize(
		resti18n.LocaleFAIR,
		"validation.header_required",
		resti18n.Params{"header": "X-Auth-Subject-Id"},
	)
	require.NoError(t, err)
	assert.Equal(t, "هدر X-Auth-Subject-Id الزامی است.", message)
}

func TestLocalizer_MissingKeyReturnsError(t *testing.T) {
	t.Parallel()

	localizer, err := New(resti18n.LocaleENUS)
	require.NoError(t, err)

	_, err = localizer.Localize(resti18n.LocaleFAIR, "missing.key", nil)
	require.Error(t, err)
}
