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

		if c.PayerType != CustomerInstallmentChargePayerType && c.PayerType != MerchantInstallmentChargePayerType && c.PayerType != PlatformInstallmentChargePayerType {
			return fmt.Errorf("invalid charge payer")
		}

		if c.RecipientType != PlatformInstallmentChargeRecipientType && c.RecipientType != MerchantInstallmentChargeRecipientType && c.RecipientType != CustomerInstallmentChargeRecipientType {
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

// ValidateCashObligations rejects financed zero/negative-net schedules until a
// generic authoritative noncash adjustment exists. A zero placeholder carrying
// no principal/upfront/customer components is not a financed obligation.
func ValidateCashObligations(installments []Installment, charges []InstallmentCharge) error {
	byID := map[InstallmentID][]InstallmentCharge{}
	for _, c := range charges {
		byID[c.InstallmentID] = append(byID[c.InstallmentID], c)
	}
	for _, i := range installments {
		if i.PrincipalAmountMinor == 0 && i.UpfrontAmountMinor == 0 && len(byID[i.ID]) == 0 {
			continue
		}
		assessment := ClassifyCustomerCash(i, byID[i.ID])
		if assessment.Kind != PositiveCashObligation {
			return fmt.Errorf("installment %s requires reconciliation of %s cash economics", i.ID, assessment.Kind)
		}
	}
	return nil
}

// CashObligationKind distinguishes collection, pending noncash recognition,
// negative economic treatment, and invalid components without inventing settlement.
type CashObligationKind string

const (
	PositiveCashObligation CashObligationKind = "positive"
	ZeroCashObligation     CashObligationKind = "zero"
	NegativeCashObligation CashObligationKind = "negative"
	InvalidCashObligation  CashObligationKind = "invalid"
)

type CashObligationAssessment struct {
	Kind        CashObligationKind
	AmountMinor uint64
	// AdjustmentAmountMinor is the magnitude of a negative net, not a payable payment.
	AdjustmentAmountMinor uint64
}

func ClassifyCustomerCash(i Installment, charges []InstallmentCharge) CashObligationAssessment {
	invalid := CashObligationAssessment{Kind: InvalidCashObligation}
	if ValidateCustomerPaymentComponents(i, charges) != nil {
		return invalid
	}
	for _, c := range charges {
		if i.Status != PaidInstallmentStatus && c.PayerType == CustomerInstallmentChargePayerType && (c.ChargeStatus == PaidInstallmentChargeStatus || ((c.ChargeType == DiscountInstallmentChargeType || c.ChargeType == WaiverInstallmentChargeType) && c.ChargeStatus == WaivedInstallmentChargeStatus)) {
			return invalid
		}
	}
	items, err := BuildPaymentAttemptItems(i, charges)
	if err != nil {
		return invalid
	}
	debits, credits, ok := paymentItemTotals(items)
	if !ok {
		return invalid
	}

	if credits > debits {
		return CashObligationAssessment{Kind: NegativeCashObligation, AdjustmentAmountMinor: credits - debits}
	}

	if credits == debits {
		return CashObligationAssessment{Kind: ZeroCashObligation}
	}
	amount := debits - credits
	if ValidatePositiveAmountMinor(amount) != nil {
		return invalid
	}
	return CashObligationAssessment{Kind: PositiveCashObligation, AmountMinor: amount}
}
