package i18n

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubLocalizer struct{}

func (stubLocalizer) Localize(locale Locale, key string, _ Params) (string, error) {
	return locale.String() + ":" + key, nil
}

func TestParseLocale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want Locale
		ok   bool
	}{
		{"en-US", LocaleENUS, true},
		{"fa-IR", LocaleFAIR, true},
		{"EN-us", LocaleENUS, true},
		{"FA-ir", LocaleFAIR, true},
		{"en", "", false},
		{"fa", "", false},
		{"en-GB", "", false},
		{"fa-AF", "", false},
		{"de", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := ParseLocale(tt.in)
			if !tt.ok {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveLocale(t *testing.T) {
	t.Parallel()

	assert.Equal(t, LocaleFAIR, ResolveLocale("fa-IR", "en-US", LocaleENUS))
	assert.Equal(t, LocaleENUS, ResolveLocale("de", "en-US", LocaleFAIR))
	assert.Equal(t, LocaleFAIR, ResolveLocale("de", "bad", LocaleFAIR))
	assert.Equal(t, LocaleFAIR, ResolveLocale("en-US;q=0.2, fa-IR;q=0.9", "", LocaleENUS))
	assert.Equal(t, LocaleENUS, ResolveLocale("fa-IR;q=0, en-US;q=1", "", LocaleFAIR))
}

func TestRequestStateAndLocalize(t *testing.T) {
	t.Parallel()

	ctx := WithRequestState(context.Background(), LocaleFAIR, stubLocalizer{})
	message, err := Localize(ctx, "message.key", nil)
	require.NoError(t, err)
	assert.Equal(t, "fa-IR:message.key", message)

	state, ok := RequestStateFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, LocaleFAIR, state.Locale)
}
