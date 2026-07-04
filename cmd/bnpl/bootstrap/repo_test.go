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
	require.NotNil(t, repos.CheckoutDecisionAllocation)
	require.NotNil(t, repos.CustomerProfile)
	require.NotNil(t, repos.CreditAccount)
	require.NotNil(t, repos.MerchantProfile)
	require.NotNil(t, repos.MerchantBranchProfile)
	require.NotNil(t, repos.MerchantTerminal)
	require.NotNil(t, repos.Installment)
	require.NotNil(t, repos.InstallmentCharge)
	require.NotNil(t, repos.PaymentAttempt)
	require.NotNil(t, repos.Inbox)
	require.NotNil(t, repos.Saga)
}
