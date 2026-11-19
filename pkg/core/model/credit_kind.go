package model

import (
	"database/sql/driver"
	"fmt"
)

// CreditKind identifies an independent BNPL credit account. A customer's
// installment capacity must never be substituted with monthly capacity (or
// conversely), even when both accounts share the same currency.
//
// Installment credit is one account regardless of whether the governing
// Checkout Policy finances three or four installments. The concrete schedule
// is a repayment-plan projection of that policy, not a second credit kind.
//
// On checkout_sessions, the empty value (SQL NULL) means Partner has not
// selected a product yet. Credit accounts always store a concrete kind.
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

// String returns the canonical persisted representation.
func (k CreditKind) String() string { return string(k) }

// EffectiveCreditKind treats omission on legacy internal surfaces as installment
// credit. Partner Create / Hosted START must not use this to invent a selection:
// an unset session credit kind means Partner SelectCreditKind has not run yet.
func EffectiveCreditKind(k CreditKind) CreditKind {
	if k == "" {
		return PayInInstallmentsCreditKind
	}

	return k
}

// Scan implements sql.Scanner. NULL maps to the unset (empty) credit kind used
// by checkout_sessions before Partner SelectCreditKind.
func (k *CreditKind) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*k = ""

		return nil

	case string:
		kind := CreditKind(v)
		if kind != "" && !kind.IsValid() {
			return fmt.Errorf("scan credit kind: unsupported value %q", v)
		}

		*k = kind

		return nil

	case []byte:
		kind := CreditKind(v)
		if kind != "" && !kind.IsValid() {
			return fmt.Errorf("scan credit kind: unsupported value %q", string(v))
		}

		*k = kind

		return nil

	default:
		return fmt.Errorf("scan credit kind: unsupported type %T", src)
	}
}

// Value implements driver.Valuer. The unset (empty) kind persists as SQL NULL
// so checkout_sessions keep the deferred-selection invariant.
func (k CreditKind) Value() (driver.Value, error) {
	if k == "" {
		return nil, nil
	}

	if !k.IsValid() {
		return nil, fmt.Errorf("value credit kind: unsupported value %q", k)
	}

	return string(k), nil
}
