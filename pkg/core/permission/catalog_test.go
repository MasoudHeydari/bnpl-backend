package permission_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

func TestCatalogParity(t *testing.T) {
	t.Parallel()

	all := permission.All()
	require.NotEmpty(t, all)

	seenCodes := make(map[string]struct{}, len(all))
	for _, perm := range all {
		require.NotEmpty(t, perm.Code, "permission code must not be empty")
		require.NotEmpty(t, perm.Name, "permission %q name must not be empty", perm.Code)
		require.NotEmpty(t, perm.Description, "permission %q description must not be empty", perm.Code)
		require.Equal(t, perm.Code, perm.String())

		_, duplicate := seenCodes[perm.Code]
		require.False(t, duplicate, "duplicate permission code %q", perm.Code)
		seenCodes[perm.Code] = struct{}{}
	}

	require.Len(t, all, 59, "expected 55 FC codes + credit-limits:create:self + 2 checkout integration manage codes + merchant-terminals:manage:merchant")
	require.Contains(t, seenCodes, "bnpl:credit-limits:create:self")
	require.Contains(t, seenCodes, "bnpl:decisions:create:self")
	require.Contains(t, seenCodes, "bnpl:contracts:approve")
	require.Contains(t, seenCodes, "bnpl:contracts:reject")
	require.Contains(t, seenCodes, "bnpl:checkout-webhooks:manage:merchant")
	require.Contains(t, seenCodes, "bnpl:checkout-return-destinations:manage:merchant")
	require.Contains(t, seenCodes, "bnpl:merchant-terminals:manage:merchant")
	require.Contains(t, seenCodes, "bnpl:merchants:read:any")
}

func TestCatalogGroupingsCoverAllWithoutDuplicates(t *testing.T) {
	t.Parallel()

	groups := [][]permission.Permission{
		permission.CustomerApp(),
		permission.MerchantPanel(),
		permission.BranchPanel(),
		permission.AdminPanel(),
	}

	seen := make(map[string]string)
	total := 0
	for i, group := range groups {
		require.NotEmpty(t, group)
		total += len(group)
		for _, perm := range group {
			if previous, ok := seen[perm.Code]; ok {
				t.Fatalf("permission %q appears in both group %s and group index %d", perm.Code, previous, i)
			}

			seen[perm.Code] = groupLabel(i)
		}
	}

	require.Equal(t, len(permission.All()), total)
	require.Len(t, seen, total)
}

func TestCatalogGettersReturnDefensiveCopies(t *testing.T) {
	t.Parallel()

	first := permission.All()
	first[0].Code = "mutated"
	second := permission.All()
	require.NotEqual(t, "mutated", second[0].Code)

	customer := permission.CustomerApp()
	customer[0].Code = "mutated-customer"
	require.NotEqual(t, "mutated-customer", permission.CustomerApp()[0].Code)
}

func groupLabel(index int) string {
	switch index {
	case 0:
		return "customerApp"

	case 1:
		return "merchantPanel"

	case 2:
		return "branchPanel"

	case 3:
		return "adminPanel"

	default:
		return "unknown"
	}
}
