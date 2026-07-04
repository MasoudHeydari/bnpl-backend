//go:build development

package catalog

import (
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout/fastdev"
)

func registeredPolicies() []checkout.Policy {
	return append(basePolicies(), fastdev.New())
}
