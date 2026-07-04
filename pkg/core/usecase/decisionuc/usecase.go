package decisionuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	coremetrics "github.com/parstis/bnpl-backend/pkg/core/metrics"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// UseCase orchestrates governed BNPL decisions.
type UseCase struct {
	pool                       repo.Pool
	decision                   repo.Decision
	customerProfile            repo.CustomerProfile
	creditAccount              repo.CreditAccount
	merchantProfile            repo.MerchantProfile
	merchantBranchProfile      repo.MerchantBranchProfile
	checkoutDecisionAllocation repo.CheckoutDecisionAllocation
	policyAssignment           repo.PolicyAssignment
	installment                repo.Installment
	installmentCharge          repo.InstallmentCharge
	outbox                     repo.Outbox
	clock                      clock.Clock
	fingerprintHasher          fingerprint.Hasher
	checkoutPolicies           *checkout.Registry
	defaultCheckoutPolicyRef   policy.Ref
	auditDestination           string
	idGenerator                identifier.Generator
	// jurisdictions gates NEW Decision creation only. Idempotent replay of an
	// existing Decision must not consult this capability.
	jurisdictions   domaincapability.OriginationJurisdictions
	businessMetrics coremetrics.BusinessMetrics
}

// New constructs a decision use case. The registry contains every compiled-in
// checkout policy version; runtime assignment chooses a ref and the code-owned
// default applies only when no explicit assignment exists.
func New(
	pool repo.Pool,
	decision repo.Decision,
	customerProfile repo.CustomerProfile,
	creditAccount repo.CreditAccount,
	merchantProfile repo.MerchantProfile,
	merchantBranchProfile repo.MerchantBranchProfile,
	checkoutDecisionAllocation repo.CheckoutDecisionAllocation,
	policyAssignment repo.PolicyAssignment,
	installment repo.Installment,
	installmentCharge repo.InstallmentCharge,
	outbox repo.Outbox,
	clk clock.Clock,
	fingerprintHasher fingerprint.Hasher,
	checkoutPolicies *checkout.Registry,
	defaultCheckoutPolicyRef policy.Ref,
	auditDestination string,
	idGenerator identifier.Generator,
	jurisdictions domaincapability.OriginationJurisdictions,
	businessMetrics coremetrics.BusinessMetrics,
) (*UseCase, error) {
	const op = "decisionuc.New"

	if pool == nil || decision == nil || customerProfile == nil ||
		creditAccount == nil ||
		merchantProfile == nil || merchantBranchProfile == nil ||
		checkoutDecisionAllocation == nil || policyAssignment == nil ||
		installment == nil || installmentCharge == nil ||
		outbox == nil || clk == nil || fingerprintHasher == nil ||
		checkoutPolicies == nil || defaultCheckoutPolicyRef.ID == "" || defaultCheckoutPolicyRef.Version == "" || auditDestination == "" || idGenerator == nil {
		return nil, fmt.Errorf("%s: decision use case dependencies are required", op)
	}

	if jurisdictions == nil {
		return nil, fmt.Errorf("%s: jurisdiction capability is required", op)
	}

	if businessMetrics == nil {
		return nil, fmt.Errorf("%s: business metrics is required", op)
	}

	return &UseCase{
		pool:                       pool,
		decision:                   decision,
		customerProfile:            customerProfile,
		creditAccount:              creditAccount,
		merchantProfile:            merchantProfile,
		merchantBranchProfile:      merchantBranchProfile,
		checkoutDecisionAllocation: checkoutDecisionAllocation,
		policyAssignment:           policyAssignment,
		installment:                installment,
		installmentCharge:          installmentCharge,
		outbox:                     outbox,
		clock:                      clk,
		fingerprintHasher:          fingerprintHasher,
		checkoutPolicies:           checkoutPolicies,
		defaultCheckoutPolicyRef:   defaultCheckoutPolicyRef,
		auditDestination:           auditDestination,
		idGenerator:                idGenerator,
		jurisdictions:              jurisdictions,
		businessMetrics:            businessMetrics,
	}, nil
}
