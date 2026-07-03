package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRepos(t *testing.T) {
	t.Parallel()

	repos := NewRepos()

	require.NotNil(t, repos.Migrations)
	require.NotNil(t, repos.Outbox)
	require.NotNil(t, repos.Health)
	require.NotNil(t, repos.Decision)
	require.NotNil(t, repos.BorrowerProfile)
	require.NotNil(t, repos.BorrowerIdentityVerification)
	require.NotNil(t, repos.BorrowerCreditLimit)
	require.NotNil(t, repos.MerchantProfile)
	require.NotNil(t, repos.MerchantBranchProfile)
}
