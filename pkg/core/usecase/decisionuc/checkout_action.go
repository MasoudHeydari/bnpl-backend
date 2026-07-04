package decisionuc

import (
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
)

// CheckoutActionType identifies the semantic action the checkout client should offer next.
type CheckoutActionType string

const (
	// CheckoutActionConfirmAndPay means contract activation requires the policy-defined initial payment.
	CheckoutActionConfirmAndPay CheckoutActionType = "confirm_and_pay"

	// CheckoutActionConfirmPurchase means confirmation creates a payment_pending
	// contract without an initial customer payment (no PaymentAttempt); Financial
	// Core merchant funding then activates it. PaymentRequired is false.
	CheckoutActionConfirmPurchase CheckoutActionType = "confirm_purchase"
)

// CheckoutAction tells clients what the customer must do after accepting an installment offer.
// UI labels remain a frontend/localization concern; this value exposes business semantics only.
type CheckoutAction struct {
	// Type is the semantic action used by the client to choose the checkout interaction.
	Type CheckoutActionType
	// PaymentRequired reports whether confirming the offer must open an initial payment flow.
	PaymentRequired bool
	// PaymentAmountMinor is the exact initial amount payable before activation, or zero when none is required.
	PaymentAmountMinor uint64
}

func checkoutActionFromContractPlan(plan checkout.ContractPlan) (CheckoutAction, error) {
	const op = "decisionuc.checkoutActionFromContractPlan"

	switch plan.ActivationMode {
	case checkout.ActivationAfterInitialPayment:
		if !plan.InitialPaymentRequired || plan.InitialPaymentAmountMinor == 0 {
			return CheckoutAction{}, fmt.Errorf("%s: initial-payment activation requires a positive initial payment", op)
		}

		return CheckoutAction{
			Type:               CheckoutActionConfirmAndPay,
			PaymentRequired:    true,
			PaymentAmountMinor: plan.InitialPaymentAmountMinor,
		}, nil

	case checkout.ActivationAfterMerchantFunding:
		if plan.InitialPaymentRequired || plan.InitialPaymentAmountMinor != 0 {
			return CheckoutAction{}, fmt.Errorf("%s: merchant-funding activation cannot require an initial payment", op)
		}

		return CheckoutAction{
			Type:               CheckoutActionConfirmPurchase,
			PaymentRequired:    false,
			PaymentAmountMinor: 0,
		}, nil

	default:
		return CheckoutAction{}, fmt.Errorf("%s: unsupported activation mode %q", op, plan.ActivationMode)
	}
}
