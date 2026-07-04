package noop

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
)

func TestResolveByMobile_IsDeterministicActive(t *testing.T) {
	t.Parallel()

	client := New()
	first, err := client.ResolveByMobile(context.Background(), extcustomer.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.NoError(t, err)

	second, err := client.ResolveByMobile(context.Background(), extcustomer.ResolveByMobileInput{
		MobileE164: "+989121234567",
	})
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Equal(t, extcustomer.ActiveStatus, first.Status)
	require.False(t, first.ExternalCustomerID.IsZero())
}

func TestResolveByMobile_RequiresMobile(t *testing.T) {
	t.Parallel()

	_, err := New().ResolveByMobile(context.Background(), extcustomer.ResolveByMobileInput{})
	require.Error(t, err)
}
