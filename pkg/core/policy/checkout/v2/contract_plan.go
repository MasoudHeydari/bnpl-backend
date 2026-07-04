package v2

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// BuildContractPlan returns v2 activation economics. V2 activates without
// collecting customer cash, claims the full financed principal as exposure,
// and asks FC to recognize that same amount as receivable.
func (p policyV2) BuildContractPlan(input checkout.ContractPlanInput) (checkout.ContractPlan, error) {
	const op = "v2.policyV2.BuildContractPlan"

	if input.FinancedAmountMinor == 0 {
		return checkout.ContractPlan{}, fmt.Errorf("%s: financed amount must be positive", op)
	}

	if input.UpfrontAmountMinor != 0 {
		return checkout.ContractPlan{}, fmt.Errorf("%s: v2 does not allow upfront amount", op)
	}

	if input.RequestedAmountMinor != input.FinancedAmountMinor {
		return checkout.ContractPlan{}, fmt.Errorf("%s: v2 requires full financing", op)
	}

	return checkout.ContractPlan{
		ActivationMode:                   p.Terms().ActivationMode,
		ScheduleAnchor:                   p.Terms().ScheduleAnchor,
		InitialPaymentRequired:           false,
		InitialPaymentAmountMinor:        0,
		ActiveExposureAmountMinor:        input.FinancedAmountMinor,
		CollectedCashAmountMinor:         0,
		ReceivableAmountMinor:            input.FinancedAmountMinor,
		FirstInstallmentPaidOnActivation: false,
	}, nil
}
