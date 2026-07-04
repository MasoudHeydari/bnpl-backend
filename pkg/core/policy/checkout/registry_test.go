package checkout_test

import (
	"errors"
	"testing"
	"time"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

var validRef = policy.Ref{ID: "checkout_contract_approval", Version: "v1"}

func newStubCheckoutPolicy(t *testing.T, ref policy.Ref) *mock.CheckoutPolicy {
	t.Helper()

	p := mock.NewCheckoutPolicy(t)
	p.EXPECT().Ref().Return(ref).Maybe()
	p.EXPECT().Terms().Return(checkout.Terms{
		InstallmentCount:              4,
		DecisionTTL:                   15 * time.Minute,
		FirstInstallmentPaymentWindow: 15 * time.Minute,
		GracePeriodDays:               5,
	}).Maybe()
	p.EXPECT().EvaluateEligibility(testifymock.Anything).Return(checkout.EligibilityResult{}).Maybe()
	p.EXPECT().EvaluateFinancing(testifymock.Anything).Return(checkout.FinancingResult{}).Maybe()
	p.EXPECT().BuildInstallmentSchedule(testifymock.Anything).Return(checkout.ScheduleResult{}, nil).Maybe()
	p.EXPECT().BuildContractPlan(testifymock.Anything).Return(checkout.ContractPlan{}, nil).Maybe()

	return p
}

func TestNewRegistry_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		policies  func(t *testing.T) []checkout.Policy
		wantError bool
	}{
		{
			name: "nil policy rejected",
			policies: func(t *testing.T) []checkout.Policy {
				t.Helper()

				return []checkout.Policy{nil}
			},
			wantError: true,
		},
		{
			name: "zero ref rejected",
			policies: func(t *testing.T) []checkout.Policy {
				t.Helper()

				return []checkout.Policy{newStubCheckoutPolicy(t, policy.Ref{})}
			},
			wantError: true,
		},
		{
			name: "empty id rejected",
			policies: func(t *testing.T) []checkout.Policy {
				t.Helper()

				return []checkout.Policy{newStubCheckoutPolicy(t, policy.Ref{ID: "", Version: "v1"})}
			},
			wantError: true,
		},
		{
			name: "empty version rejected",
			policies: func(t *testing.T) []checkout.Policy {
				t.Helper()

				return []checkout.Policy{newStubCheckoutPolicy(t, policy.Ref{ID: "checkout_contract_approval", Version: ""})}
			},
			wantError: true,
		},
		{
			name: "duplicate ref rejected",
			policies: func(t *testing.T) []checkout.Policy {
				t.Helper()

				return []checkout.Policy{
					newStubCheckoutPolicy(t, validRef),
					newStubCheckoutPolicy(t, validRef),
				}
			},
			wantError: true,
		},
		{
			name: "known ref accepted",
			policies: func(t *testing.T) []checkout.Policy {
				t.Helper()

				return []checkout.Policy{newStubCheckoutPolicy(t, validRef)}
			},
			wantError: false,
		},
		{
			name: "empty registration accepted",
			policies: func(t *testing.T) []checkout.Policy {
				t.Helper()

				return nil
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registry, err := checkout.NewRegistry(tt.policies(t)...)
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

	registry, err := checkout.NewRegistry(newStubCheckoutPolicy(t, validRef))
	require.NoError(t, err)

	resolved, err := registry.Resolve(validRef)
	require.NoError(t, err)
	require.Equal(t, validRef, resolved.Ref())
}

func TestRegistry_ResolveUnknownRefReturnsErrPolicyNotFound(t *testing.T) {
	t.Parallel()

	registry, err := checkout.NewRegistry(newStubCheckoutPolicy(t, validRef))
	require.NoError(t, err)

	unknownRef := policy.Ref{ID: validRef.ID, Version: "v2"}

	_, resolveErr := registry.Resolve(unknownRef)
	require.Error(t, resolveErr)
	require.True(t, errors.Is(resolveErr, checkout.ErrPolicyNotFound))
	require.Contains(t, resolveErr.Error(), "checkout_contract_approval/v2")
}

func TestRegistry_ResolveMatchesBothIDAndVersion(t *testing.T) {
	t.Parallel()

	refOtherID := policy.Ref{ID: "other_policy", Version: "v9"}

	registry, err := checkout.NewRegistry(
		newStubCheckoutPolicy(t, validRef),
		newStubCheckoutPolicy(t, refOtherID),
	)
	require.NoError(t, err)

	resolved, err := registry.Resolve(validRef)
	require.NoError(t, err)
	require.Equal(t, validRef, resolved.Ref())

	resolved, err = registry.Resolve(refOtherID)
	require.NoError(t, err)
	require.Equal(t, refOtherID, resolved.Ref())

	// validRef.ID paired with refOtherID.Version was never registered: a
	// matching ID alone must not be sufficient to resolve.
	_, err = registry.Resolve(policy.Ref{ID: validRef.ID, Version: refOtherID.Version})
	require.Error(t, err)
}

func TestRegistry_ResolveOnNilRegistryReturnsErrorNotPanic(t *testing.T) {
	t.Parallel()

	var registry *checkout.Registry

	_, err := registry.Resolve(validRef)
	require.Error(t, err)
	require.True(t, errors.Is(err, checkout.ErrPolicyNotFound))
}
