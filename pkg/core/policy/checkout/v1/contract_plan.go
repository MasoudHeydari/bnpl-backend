package v1

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// BuildContractPlan returns v1 activation economics. V1 requires upfront plus
// first principal to be collected before activation; only the remaining
// financed principal becomes active exposure/receivable afterwards.
func (p policyV1) BuildContractPlan(input checkout.ContractPlanInput) (checkout.ContractPlan, error) {
	const op = "v1.policyV1.BuildContractPlan"

	if input.FinancedAmountMinor == 0 {
		return checkout.ContractPlan{}, fmt.Errorf("%s: financed amount must be positive", op)
	}

	if input.FirstInstallmentPrincipalAmountMinor > input.FinancedAmountMinor {
		return checkout.ContractPlan{}, fmt.Errorf("%s: first installment principal exceeds financed amount", op)
	}

	initialPayment, err := model.AddAmountMinor(input.UpfrontAmountMinor, input.FirstInstallmentPrincipalAmountMinor)
	if err != nil {
		return checkout.ContractPlan{}, fmt.Errorf("%s: initial payment: %w", op, err)
	}

	if initialPayment != input.FirstInstallmentTotalAmountMinor {
		return checkout.ContractPlan{}, fmt.Errorf("%s: first installment total does not equal upfront plus first principal", op)
	}

	remainingPrincipal, err := model.SubtractAmountMinor(input.FinancedAmountMinor, input.FirstInstallmentPrincipalAmountMinor)
	if err != nil {
		return checkout.ContractPlan{}, fmt.Errorf("%s: remaining principal: %w", op, err)
	}

	return checkout.ContractPlan{
		ActivationMode:                   p.Terms().ActivationMode,
		ScheduleAnchor:                   p.Terms().ScheduleAnchor,
		InitialPaymentRequired:           true,
		InitialPaymentAmountMinor:        initialPayment,
		ActiveExposureAmountMinor:        remainingPrincipal,
		CollectedCashAmountMinor:         initialPayment,
		ReceivableAmountMinor:            remainingPrincipal,
		FirstInstallmentPaidOnActivation: true,
	}, nil
}
