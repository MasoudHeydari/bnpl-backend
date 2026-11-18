package checkout

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// RepaymentPlan projects the installment repayment-plan identifier from an
// immutable Checkout Policy's terms. Monthly credit returns ok=false because it
// keeps its existing single-payment API representation without a pay_in_N label.
//
// Callers must resolve the persisted policy Ref first; never infer the plan from
// CreditKind alone or from a mutable Policy Assignment that may have changed.
func RepaymentPlan(terms Terms) (model.RepaymentPlanKind, bool, error) {
	const op = "checkout.RepaymentPlan"

	kind := model.EffectiveCreditKind(terms.CreditKind)
	switch kind {
	case model.PayNextMonthCreditKind:
		return "", false, nil

	case model.PayInInstallmentsCreditKind:
		plan, err := model.RepaymentPlanFromInstallmentCount(terms.InstallmentCount)
		if err != nil {
			return "", false, fmt.Errorf("%s: %w", op, err)
		}

		return plan, true, nil

	default:
		return "", false, fmt.Errorf("%s: unsupported credit kind %q", op, kind)
	}
}
