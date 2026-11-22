package v1_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	checkoutverificationv1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkoutverification/v1"
)

func TestPolicy_V1Terms(t *testing.T) {
	t.Parallel()

	p := checkoutverificationv1.New()
	require.NoError(t, p.Validate())
	require.Equal(t, "checkout_merchant_verification", p.ID())
	require.Equal(t, "v1", p.Version())
	require.Equal(t, 30*time.Minute, checkoutverificationv1.VerifyTimeout)
}

func TestPolicy_DeadlineAt(t *testing.T) {
	t.Parallel()

	p := checkoutverificationv1.New()
	pendingAt := time.Date(2026, 11, 22, 14, 0, 0, 0, time.UTC)
	require.Equal(t, pendingAt.Add(30*time.Minute), p.DeadlineAt(pendingAt))
}

func TestPolicy_AllowsVerifyAt(t *testing.T) {
	t.Parallel()

	p := checkoutverificationv1.New()
	deadline := time.Date(2026, 11, 22, 14, 30, 0, 0, time.UTC)

	require.True(t, p.AllowsVerifyAt(deadline.Add(-time.Second), deadline))
	require.False(t, p.AllowsVerifyAt(deadline, deadline))
	require.False(t, p.AllowsVerifyAt(deadline.Add(time.Second), deadline))
}
