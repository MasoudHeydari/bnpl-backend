package model

// CreditKind identifies an independent BNPL credit kind. A customer's
// installment capacity must never be substituted with monthly capacity (or
// conversely), even when both accounts share the same currency.
type CreditKind string

const (
	// PayIn4CreditKind is installment credit. The actual installment count is
	// determined by the persisted Checkout Policy (trusted v2 uses three).
	PayIn4CreditKind CreditKind = "pay_in_4"
	// PayNextMonthCreditKind is credit repaid on the first of the next Jalali month.
	PayNextMonthCreditKind CreditKind = "pay_next_month"
)

// IsValid reports whether a credit kind is supported by BNPL.
func (k CreditKind) IsValid() bool {
	switch k {
	case PayIn4CreditKind, PayNextMonthCreditKind:
		return true

	default:
		return false
	}
}

// EffectiveCreditKind treats omission on legacy internal surfaces as pay-in-four.
// Partner Create / Hosted START must not use this to invent a selection: an
// unset session credit kind means Partner SelectCreditKind has not run yet.
func EffectiveCreditKind(k CreditKind) CreditKind {
	if k == "" {
		return PayIn4CreditKind
	}

	return k
}
