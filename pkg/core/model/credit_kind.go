package model

// CreditKind identifies an independent BNPL credit account. A customer's
// installment capacity must never be substituted with monthly capacity (or
// conversely), even when both accounts share the same currency.
//
// Installment credit is one account regardless of whether the governing
// Checkout Policy finances three or four installments. The concrete schedule
// is a repayment-plan projection of that policy, not a second credit kind.
type CreditKind string

const (
	// PayInInstallmentsCreditKind is installment credit. The actual installment
	// count and repayment plan are determined by the persisted Checkout Policy
	// (ordinary v1 uses four; trusted v2 uses three).
	PayInInstallmentsCreditKind CreditKind = "pay_in_installments"
	// PayNextMonthCreditKind is credit repaid on the first of the next Jalali month.
	PayNextMonthCreditKind CreditKind = "pay_next_month"
)

// IsValid reports whether a credit kind is supported by BNPL.
func (k CreditKind) IsValid() bool {
	switch k {
	case PayInInstallmentsCreditKind, PayNextMonthCreditKind:
		return true

	default:
		return false
	}
}

// EffectiveCreditKind treats omission on legacy internal surfaces as installment
// credit. Partner Create / Hosted START must not use this to invent a selection:
// an unset session credit kind means Partner SelectCreditKind has not run yet.
func EffectiveCreditKind(k CreditKind) CreditKind {
	if k == "" {
		return PayInInstallmentsCreditKind
	}

	return k
}
