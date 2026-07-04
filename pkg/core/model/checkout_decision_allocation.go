package model

import (
	"time"

	"github.com/google/uuid"
)

// CheckoutDecisionAllocation persists the material financial result of a checkout decision.
type CheckoutDecisionAllocation struct {
	// DecisionID uniquely identifies the checkout decision this allocation belongs to.
	DecisionID DecisionID `db:"decision_id" json:"decision_id"`

	// CheckoutKey is the merchant checkout identity associated with the decision.
	CheckoutKey string `db:"checkout_key" json:"checkout_key"`

	// CustomerID is the internal customer profile primary key for this allocation.
	CustomerID CustomerID `db:"customer_id" json:"customer_id"`

	// MerchantProfileID identifies the merchant BNPL profile for this allocation.
	MerchantProfileID MerchantProfileID `db:"merchant_profile_id" json:"merchant_profile_id"`

	// MerchantBranchProfileID optionally identifies the branch profile used for checkout.
	MerchantBranchProfileID *MerchantBranchProfileID `db:"merchant_branch_profile_id" json:"merchant_branch_profile_id,omitempty"`

	// PurchaseChannel identifies whether the checkout originated from POS or online sales.
	PurchaseChannel PurchaseChannel `db:"purchase_channel" json:"purchase_channel"`

	// PurchaseSourceID is the external POS/GPS or online-store UUID used to resolve the selling point.
	PurchaseSourceID uuid.UUID `db:"purchase_source_id" json:"purchase_source_id"`

	// MerchantOrderID optionally identifies the corresponding order in the merchant online system.
	MerchantOrderID *string `db:"merchant_order_id" json:"merchant_order_id,omitempty"`

	// RequestedAmountMinor is the original checkout request amount in minor units.
	RequestedAmountMinor uint64 `db:"requested_amount_minor" json:"requested_amount_minor"`

	// ApprovedFinancedAmountMinor is the financed amount approved for the customer.
	ApprovedFinancedAmountMinor uint64 `db:"approved_financed_amount_minor" json:"approved_financed_amount_minor"`

	// UpfrontAmountMinor is the amount the customer must pay up front.
	UpfrontAmountMinor uint64 `db:"upfront_amount_minor" json:"upfront_amount_minor"`

	// AvailableCreditMinor is the evaluation-time available-credit snapshot; a Decision does not reserve exposure.
	AvailableCreditMinor uint64 `db:"available_credit_minor" json:"available_credit_minor"`

	// Currency is the currency of all monetary fields on this allocation.
	Currency Currency `db:"currency" json:"currency"`

	// InstallmentCount is the number of installments in the approved schedule.
	InstallmentCount int `db:"installment_count" json:"installment_count"`

	// ExpiresAt is when this allocation decision expires.
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`

	// CreatedAt is when the allocation row was created.
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
