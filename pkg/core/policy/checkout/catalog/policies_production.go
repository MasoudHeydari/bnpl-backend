//go:build production

package catalog

import "github.com/parstis/bnpl-backend/pkg/core/policy/checkout"

func registeredPolicies() []checkout.Policy {
	return basePolicies()
}
