package model

import (
	"time"

	"github.com/google/uuid"
)

// CheckoutFinancingTermStatus identifies the lifecycle state of checkout
// financing terms produced by a checkout decision.
type CheckoutFinancingTermStatus string

const (
	// CheckoutFinancingTermOffered indicates financing terms are available for
	// the borrower to accept.
	CheckoutFinancingTermOffered CheckoutFinancingTermStatus = "offered"

	// CheckoutFinancingTermExpired indicates the offer window has elapsed.
	CheckoutFinancingTermExpired CheckoutFinancingTermStatus = "expired"

	// CheckoutFinancingTermCancelled indicates the offer was cancelled before
	// acceptance.
	CheckoutFinancingTermCancelled CheckoutFinancingTermStatus = "cancelled"

	// CheckoutFinancingTermConvertedToContract indicates the offer was accepted
	// and converted into a pending contract.
	CheckoutFinancingTermConvertedToContract CheckoutFinancingTermStatus = "converted_to_contract"
)

// CheckoutFinancingTerm stores the calculated financing breakdown for a
// checkout decision.
type CheckoutFinancingTerm struct {
	ID uuid.UUID `db:"id" json:"id"`

	CheckoutIntentID uuid.UUID `db:"checkout_intent_id" json:"checkout_intent_id"`
	DecisionID       uuid.UUID `db:"decision_id" json:"decision_id"`

	BorrowerProfileID       uuid.UUID  `db:"borrower_profile_id" json:"borrower_profile_id"`
	MerchantProfileID       uuid.UUID  `db:"merchant_profile_id" json:"merchant_profile_id"`
	MerchantBranchProfileID *uuid.UUID `db:"merchant_branch_profile_id" json:"merchant_branch_profile_id,omitempty"`

	RequestedAmountMinor          uint64 `db:"requested_amount_minor" json:"requested_amount_minor"`
	FinancedAmountMinor           uint64 `db:"financed_amount_minor" json:"financed_amount_minor"`
	UpfrontAmountMinor            uint64 `db:"upfront_amount_minor" json:"upfront_amount_minor"`
	RegularInstallmentAmountMinor uint64 `db:"regular_installment_amount_minor" json:"regular_installment_amount_minor"`
	FirstInstallmentAmountMinor   uint64 `db:"first_installment_amount_minor" json:"first_installment_amount_minor"`

	RegularInstallmentPrincipalAmountMinor uint64 `db:"regular_installment_principal_amount_minor" json:"regular_installment_principal_amount_minor"`
	FirstInstallmentPrincipalAmountMinor   uint64 `db:"first_installment_principal_amount_minor" json:"first_installment_principal_amount_minor"`
	FirstInstallmentUpfrontAmountMinor     uint64 `db:"first_installment_upfront_amount_minor" json:"first_installment_upfront_amount_minor"`
	FirstInstallmentTotalAmountMinor       uint64 `db:"first_installment_total_amount_minor" json:"first_installment_total_amount_minor"`

	TotalFeeAmountMinor     uint64 `db:"total_fee_amount_minor" json:"total_fee_amount_minor"`
	TotalPayableAmountMinor uint64 `db:"total_payable_amount_minor" json:"total_payable_amount_minor"`

	Currency         Currency `db:"currency" json:"currency,omitempty"`
	InstallmentCount int      `db:"installment_count" json:"installment_count"`

	Status    CheckoutFinancingTermStatus `db:"status" json:"status,omitempty"`
	ExpiresAt time.Time                   `db:"expires_at" json:"expires_at"`
	CreatedAt time.Time                   `db:"created_at" json:"created_at"`
	UpdatedAt time.Time                   `db:"updated_at" json:"updated_at"`
}
