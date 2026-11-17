package model

// CreditKind identifies an independent BNPL credit product. A customer's
// installment capacity must never be substituted with monthly capacity (or
// conversely), even when both accounts share the same currency.
type CreditKind string

const (
	// PayIn4CreditKind is credit repaid in four installments.
	PayIn4CreditKind CreditKind = "pay_in_4"
	// PayNextMonthCreditKind is credit repaid on the fifth of the next Jalali month.
	PayNextMonthCreditKind CreditKind = "pay_next_month"
)

// IsValid reports whether a credit product is supported by BNPL.
func (k CreditKind) IsValid() bool {
	switch k {
	case PayIn4CreditKind, PayNextMonthCreditKind:
		return true
	default:
		return false
	}
}

// EffectiveCreditKind treats omission on legacy API surfaces as pay-in-four.
// New product-aware commands should require an explicit kind at their boundary.
func EffectiveCreditKind(k CreditKind) CreditKind {
	if k == "" {
		return PayIn4CreditKind
	}
	return k
}
