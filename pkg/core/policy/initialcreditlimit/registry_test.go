package initialcreditlimit_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
)

var validRef = policy.Ref{ID: "customer_default_initial_limit", Version: "v1"}

func newStubInitialCreditLimitPolicy(t *testing.T, ref policy.Ref) *mock.InitialCreditLimitPolicy {
	t.Helper()

	p := mock.NewInitialCreditLimitPolicy(t)
	p.EXPECT().Ref().Return(ref).Maybe()
	p.EXPECT().Terms().Return(initialcreditlimit.Terms{
		DefaultAmountMinor: 100_000_000,
		MaxAmountMinor:     2_000_000_000,
		Currency:           model.IRRCurrency,
		ValidFor:           365 * 24 * time.Hour,
		ApprovalReasonCode: "INITIAL_CREDIT_LIMIT_POLICY_APPROVED",
	}).Maybe()

	return p
}

func TestNewRegistry_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		policies  func(t *testing.T) []initialcreditlimit.Policy
		wantError bool
	}{
		{
			name: "nil policy rejected",
			policies: func(t *testing.T) []initialcreditlimit.Policy {
				t.Helper()

				return []initialcreditlimit.Policy{nil}
			},
			wantError: true,
		},
		{
			name: "zero ref rejected",
			policies: func(t *testing.T) []initialcreditlimit.Policy {
				t.Helper()

				return []initialcreditlimit.Policy{newStubInitialCreditLimitPolicy(t, policy.Ref{})}
			},
			wantError: true,
		},
		{
			name: "empty id rejected",
			policies: func(t *testing.T) []initialcreditlimit.Policy {
				t.Helper()

				return []initialcreditlimit.Policy{newStubInitialCreditLimitPolicy(t, policy.Ref{ID: "", Version: "v1"})}
			},
			wantError: true,
		},
		{
			name: "empty version rejected",
			policies: func(t *testing.T) []initialcreditlimit.Policy {
				t.Helper()

				return []initialcreditlimit.Policy{newStubInitialCreditLimitPolicy(t, policy.Ref{ID: validRef.ID, Version: ""})}
			},
			wantError: true,
		},
		{
			name: "duplicate ref rejected",
			policies: func(t *testing.T) []initialcreditlimit.Policy {
				t.Helper()

				return []initialcreditlimit.Policy{
					newStubInitialCreditLimitPolicy(t, validRef),
					newStubInitialCreditLimitPolicy(t, validRef),
				}
			},
			wantError: true,
		},
		{
			name: "known ref accepted",
			policies: func(t *testing.T) []initialcreditlimit.Policy {
				t.Helper()

				return []initialcreditlimit.Policy{newStubInitialCreditLimitPolicy(t, validRef)}
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registry, err := initialcreditlimit.NewRegistry(tt.policies(t)...)
			if tt.wantError {
				require.Error(t, err)
				require.Nil(t, registry)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, registry)
		})
	}
}

func TestRegistry_ResolveKnownRef(t *testing.T) {
	t.Parallel()

	registry, err := initialcreditlimit.NewRegistry(newStubInitialCreditLimitPolicy(t, validRef))
	require.NoError(t, err)

	resolved, err := registry.Resolve(validRef)
	require.NoError(t, err)
	require.Equal(t, validRef, resolved.Ref())
}

func TestRegistry_ResolveUnknownRefReturnsErrPolicyNotFound(t *testing.T) {
	t.Parallel()

	registry, err := initialcreditlimit.NewRegistry(newStubInitialCreditLimitPolicy(t, validRef))
	require.NoError(t, err)

	_, resolveErr := registry.Resolve(policy.Ref{ID: validRef.ID, Version: "v2"})
	require.Error(t, resolveErr)
	require.True(t, errors.Is(resolveErr, initialcreditlimit.ErrPolicyNotFound))
}

func TestRegistry_ResolveOnNilRegistryReturnsErrorNotPanic(t *testing.T) {
	t.Parallel()

	var registry *initialcreditlimit.Registry

	_, err := registry.Resolve(validRef)
	require.Error(t, err)
	require.True(t, errors.Is(err, initialcreditlimit.ErrPolicyNotFound))
}
