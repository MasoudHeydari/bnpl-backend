package v3

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// BuildContractPlan recognizes the full merchant-funded purchase in FC before
// consuming monthly exposure. No customer payment is due at activation.
func (p policyV3) BuildContractPlan(input checkout.ContractPlanInput) (checkout.ContractPlan, error) {
	const op = "v3.policyV3.BuildContractPlan"
	if input.FinancedAmountMinor == 0 || input.UpfrontAmountMinor != 0 ||
		input.RequestedAmountMinor != input.FinancedAmountMinor ||
		input.FirstInstallmentPrincipalAmountMinor != input.FinancedAmountMinor ||
		input.FirstInstallmentTotalAmountMinor != input.FinancedAmountMinor {
		return checkout.ContractPlan{}, fmt.Errorf("%s: invalid monthly financing allocation", op)
	}
	return checkout.ContractPlan{
		ActivationMode: checkout.ActivationAfterMerchantFunding,
		ScheduleAnchor: checkout.ScheduleFromContractActivation,
		InitialPaymentRequired: false,
		InitialPaymentAmountMinor: 0,
		ActiveExposureAmountMinor: input.FinancedAmountMinor,
		CollectedCashAmountMinor: 0,
		ReceivableAmountMinor: input.FinancedAmountMinor,
		FirstInstallmentPaidOnActivation: false,
	}, nil
}
