package noop

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	extidentity "github.com/parstis/bnpl-backend/pkg/core/external/identity"
)

func TestResolveByMobile_IsDeterministicIndividualActive(t *testing.T) {
	t.Parallel()

	client := New()
	first, err := client.ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.NoError(t, err)

	second, err := client.ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Equal(t, extidentity.IndividualSubjectType, first.SubjectType)
	require.Equal(t, extidentity.ActiveSubjectStatus, first.SubjectStatus)
	require.False(t, first.IdentitySubjectID.IsZero())
}

func TestResolveByMobile_RequiresMobile(t *testing.T) {
	t.Parallel()

	_, err := New().ResolveByMobile(context.Background(), extidentity.ResolveByMobileInput{})
	require.Error(t, err)
}
