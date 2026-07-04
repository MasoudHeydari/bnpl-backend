package returndestination_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
)

func TestValidateDestinationName_Valid(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"default",
		"damz-web",
		"mobile-app",
		"mobile-app-v2",
		"checkout-v2",
		"storefront-1",
		"a",
		"a1",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, returndestination.ValidateDestinationName(name))
		})
	}
}

func TestValidateDestinationName_Invalid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want error
	}{
		{name: "", want: returndestination.ErrDestinationNameEmpty},
		{name: " ", want: returndestination.ErrDestinationNameInvalid},
		{name: "Damz", want: returndestination.ErrDestinationNameInvalid},
		{name: "DAMZ", want: returndestination.ErrDestinationNameInvalid},
		{name: "Damz-Web", want: returndestination.ErrDestinationNameInvalid},
		{name: "damz_web", want: returndestination.ErrDestinationNameInvalid},
		{name: "damz web", want: returndestination.ErrDestinationNameInvalid},
		{name: "-damz", want: returndestination.ErrDestinationNameInvalid},
		{name: "damz-", want: returndestination.ErrDestinationNameInvalid},
		{name: "damz--web", want: returndestination.ErrDestinationNameInvalid},
		{name: "foo/bar", want: returndestination.ErrDestinationNameInvalid},
		{name: "../admin", want: returndestination.ErrDestinationNameInvalid},
		{name: "foo@bar", want: returndestination.ErrDestinationNameInvalid},
		{name: "!!!!", want: returndestination.ErrDestinationNameInvalid},
		{name: "سلام", want: returndestination.ErrDestinationNameInvalid},
		{name: strings.Repeat("a", 65), want: returndestination.ErrDestinationNameTooLong},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.ErrorIs(t, returndestination.ValidateDestinationName(tc.name), tc.want)
		})
	}
}

func TestValidateDestinationName_DoesNotLowercase(t *testing.T) {
	t.Parallel()

	require.ErrorIs(
		t,
		returndestination.ValidateDestinationName("Damz-Web"),
		returndestination.ErrDestinationNameInvalid,
	)
}
