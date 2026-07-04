package model_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestParseTerminalID_CanonicalOnly(t *testing.T) {
	t.Parallel()

	id, err := model.ParseTerminalID("5839172648103748291")
	require.NoError(t, err)
	require.Equal(t, model.TerminalID(5839172648103748291), id)

	id, err = model.ParseTerminalID("4242")
	require.NoError(t, err)
	require.Equal(t, model.TerminalID(4242), id)

	rejects := map[string]string{
		"empty":             "",
		"zero":              "0",
		"negative":          "-1",
		"leading plus":      "+4242",
		"leading zero":      "04242",
		"surrounding space": " 4242 ",
		"trailing space":    "4242 ",
		"leading space":     " 4242",
		"non numeric":       "not-a-number",
		"decimal":           "42.0",
	}

	for name, raw := range rejects {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := model.ParseTerminalID(raw)
			require.Error(t, err)
		})
	}
}
