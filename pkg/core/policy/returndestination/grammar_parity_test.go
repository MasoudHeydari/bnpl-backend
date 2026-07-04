package returndestination_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
)

// postgresDestinationNamePattern is the CHECK constraint expression embedded in
// 000_schema.sql for merchant_return_destinations.destination_name and
// checkout_sessions.return_destination. It must accept exactly the same strings
// as returndestination.ValidateDestinationName.
var postgresDestinationNamePattern = regexp.MustCompile(returndestination.DestinationNamePattern)

func TestDestinationNameGrammar_CoreMatchesPostgresPattern(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		valid bool
	}{
		{name: "default", valid: true},
		{name: "damz-web", valid: true},
		{name: "a", valid: true},
		{name: "a1", valid: true},
		{name: "storefront-1", valid: true},
		{name: "mobile-app-v2", valid: true},
		{name: "Damz", valid: false},
		{name: "damz_web", valid: false},
		{name: "damz--web", valid: false},
		{name: "-damz", valid: false},
		{name: "damz-", valid: false},
		{name: "foo/bar", valid: false},
		{name: "foo bar", valid: false},
		{name: "سلام", valid: false},
		{name: "../foo", valid: false},
		{name: strings.Repeat("a", 65), valid: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			coreErr := returndestination.ValidateDestinationName(tc.name)
			dbMatch := len(tc.name) >= 1 &&
				len(tc.name) <= returndestination.MaxDestinationNameLength &&
				postgresDestinationNamePattern.MatchString(tc.name)

			if tc.valid {
				require.NoError(t, coreErr)
				require.True(t, dbMatch)
				return
			}

			require.Error(t, coreErr)
			// Length-only failures are rejected by Core before the regex; the DB
			// length CHECK is separate. For grammar cases, Core invalidity and
			// regex non-match must agree.
			if len(tc.name) >= 1 && len(tc.name) <= returndestination.MaxDestinationNameLength {
				require.False(t, postgresDestinationNamePattern.MatchString(tc.name))
			}
		})
	}
}
