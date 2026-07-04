package v1_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	v1 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v1"
)

func TestPolicyV1_StableIdentityAndLiterals(t *testing.T) {
	t.Parallel()

	p := v1.New()
	terms := p.Terms()

	require.Equal(t, policy.Ref{ID: "customer_default_initial_limit", Version: "v1"}, p.Ref())
	require.Equal(t, uint64(100_000_000), terms.DefaultAmountMinor)
	require.Equal(t, uint64(2_000_000_000), terms.MaxAmountMinor)
	require.Equal(t, model.IRRCurrency, terms.Currency)
	require.Equal(t, 365*24*time.Hour, terms.ValidFor)
	require.Equal(t, "INITIAL_CREDIT_LIMIT_POLICY_APPROVED", terms.ApprovalReasonCode)
}

func TestPolicyV1_DeterministicForRepeatedCalls(t *testing.T) {
	t.Parallel()

	p := v1.New()

	require.Equal(t, p.Ref(), v1.New().Ref())
	require.Equal(t, p.Terms(), v1.New().Terms())
}
