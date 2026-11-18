package model

import "fmt"

// RepaymentPlanKind identifies the installment repayment arrangement derived
// from an immutable Checkout Policy. It is not a credit-account identity and
// must not be confused with a repayment-plan aggregate, schedule row, or
// database primary key.
//
// Only installment credit (pay_in_installments) uses these values. Monthly
// credit keeps its existing single-payment API representation without a
// pay_in_N repayment-plan label.
type RepaymentPlanKind string

const (
	// PayIn3RepaymentPlan is the trusted three-installment arrangement (Checkout Policy v2).
	PayIn3RepaymentPlan RepaymentPlanKind = "pay_in_3"
	// PayIn4RepaymentPlan is the ordinary four-installment arrangement (Checkout Policy v1).
	PayIn4RepaymentPlan RepaymentPlanKind = "pay_in_4"
)

// IsValid reports whether the repayment-plan identifier is supported.
func (k RepaymentPlanKind) IsValid() bool {
	switch k {
	case PayIn3RepaymentPlan, PayIn4RepaymentPlan:
		return true

	default:
		return false
	}
}

// RepaymentPlanFromInstallmentCount maps an installment Checkout Policy's
// schedule length onto the repayment-plan identifier. Callers must first
// confirm the governing policy finances installment credit.
func RepaymentPlanFromInstallmentCount(installmentCount int) (RepaymentPlanKind, error) {
	const op = "model.RepaymentPlanFromInstallmentCount"

	switch installmentCount {
	case 3:
		return PayIn3RepaymentPlan, nil

	case 4:
		return PayIn4RepaymentPlan, nil

	default:
		return "", fmt.Errorf(
			"%s: installment count %d has no repayment-plan identifier",
			op,
			installmentCount,
		)
	}
}

// FinancingIdentityFromInstallmentCount projects credit-kind and repayment-plan
// labels from a persisted contractual installment count. Monthly single-payment
// contracts (count 1) omit a pay_in_N repayment-plan label.
func FinancingIdentityFromInstallmentCount(
	installmentCount int,
) (CreditKind, RepaymentPlanKind, error) {
	const op = "model.FinancingIdentityFromInstallmentCount"

	switch installmentCount {
	case 1:
		return PayNextMonthCreditKind, "", nil

	case 3, 4:
		plan, err := RepaymentPlanFromInstallmentCount(installmentCount)
		if err != nil {
			return "", "", fmt.Errorf("%s: %w", op, err)
		}

		return PayInInstallmentsCreditKind, plan, nil

	default:
		return "", "", fmt.Errorf(
			"%s: installment count %d has no financing identity",
			op,
			installmentCount,
		)
	}
}
