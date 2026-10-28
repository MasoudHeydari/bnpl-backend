package model

import "fmt"

// ValidateCustomerPaymentComponents validates persisted economic facts before
// snapshotting. Merchant-paid components do not enter the customer's payment.
func ValidateCustomerPaymentComponents(i Installment, charges []InstallmentCharge) error {
	if i.ID.IsZero() || !i.Currency.IsValid() {
		return fmt.Errorf("invalid installment identity or currency")
	}
	seen := map[InstallmentChargeID]bool{}
	for _, c := range charges {
		if c.InstallmentID != i.ID || c.Currency != i.Currency || !c.ChargeStatus.IsValid() {
			return fmt.Errorf("payment component identity, currency or status mismatch")
		}
		if c.PayerType != CustomerInstallmentChargePayerType && c.PayerType != MerchantInstallmentChargePayerType {
			return fmt.Errorf("invalid charge payer")
		}
		if c.RecipientType != PlatformInstallmentChargeRecipientType && c.RecipientType != MerchantInstallmentChargeRecipientType {
			return fmt.Errorf("invalid charge recipient")
		}
		if _, err := c.ChargeType.CustomerPaymentChargeEffect(); err != nil {
			return err
		}
		if c.ID.IsZero() || seen[c.ID] {
			return fmt.Errorf("duplicate or missing charge identity")
		}
		seen[c.ID] = true
		if err := ValidatePositiveAmountMinor(c.AmountMinor); err != nil {
			return err
		}
	}
	return nil
}

// IsChargeOnlyPayment identifies an immutable residual-charge payment. Its absence
// of principal prevents a historical principal settlement from being applied twice.
func IsChargeOnlyPayment(items []PaymentAttemptItem) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if item.ItemType != ChargePaymentAttemptItemType {
			return false
		}
	}
	return true
}

// OutstandingPrincipalForPayment excludes principal already settled historically.
func OutstandingPrincipalForPayment(i Installment) uint64 {
	if i.Status == PaidInstallmentStatus {
		return 0
	}
	return i.PrincipalAmountMinor
}
