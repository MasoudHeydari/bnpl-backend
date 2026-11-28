package v1

import (
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/merchantprovisioning"
)

type policyV1 struct{}

// New returns the compiled onboarding policy, independent of deployment.
func New() merchantprovisioning.Policy { return policyV1{} }

// Ref returns the v1 Merchant provisioning policy identity.
func (policyV1) Ref() policy.Ref {
	return policy.Ref{
		ID:      "merchant_provisioning",
		Version: "v1",
	}
}

// Evaluate validates the declared limits and returns the accepted account terms.
func (policyV1) Evaluate(declaration model.MerchantSelfDeclaration) (merchantprovisioning.Terms, error) {
	// A zero limit must not accidentally acquire the existing unlimited semantics.
	// The platform ceilings also fit the PostgreSQL bigint/integer columns.
	// Accepted declarations are preserved without clamping; checkout policies still apply.
	if declaration.MaxAmountMinor == 0 ||
		declaration.MaxAmountMinor > platformMaxAmountMinor ||
		declaration.MaxTenureMonths == 0 ||
		declaration.MaxTenureMonths > platformMaxTenureMonths ||
		declaration.MaxAmountCurrency != model.IRRCurrency {
		return merchantprovisioning.Terms{}, merchantprovisioning.ErrInvalidDeclaration
	}

	// Keep the previously selected pilot onboarding terms, now in a versioned
	// policy. They do not vary by deployment and are not merchant declarations.
	return merchantprovisioning.Terms{
		Limits:           declaration,
		FundingModels:    []model.FundingModel{model.PlatformFundedFundingModel},
		SettlementModels: []model.SettlementModel{model.DirectSettlementModel},
		Currencies:       []model.Currency{model.IRRCurrency},
		RiskTier:         model.LowRiskTier,
		Jurisdiction:     model.IRJurisdiction,
		CreditKinds:      []model.CreditKind{model.PayInInstallmentsCreditKind},
	}, nil
}
