package contractuc

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ActivateFromFinancialCorePaymentResultInput carries Financial Core payment result fields
// required to activate a payment_pending contract after first installment payment.
type ActivateFromFinancialCorePaymentResultInput struct {
	// ContractID identifies the contract.
	ContractID model.ContractID

	// InvoiceID identifies the Financial Core invoice.
	InvoiceID string
	// PaymentIntentID identifies the Financial Core payment intent.
	PaymentIntentID string
	// FinancialPaymentAttemptID identifies the Financial Core payment attempt.
	FinancialPaymentAttemptID *string
	// PaymentSessionID identifies the Financial Core payment session.
	PaymentSessionID string

	// PaidAmountMinor is the paid amount in minor currency units.
	PaidAmountMinor uint64
	// Currency is the ISO currency for monetary amounts.
	Currency model.Currency
	// PaidAt is when payment occurred, if applicable.
	PaidAt time.Time

	// EventID uniquely identifies the inbound event.
	EventID string
	// EventOccurredAt is when the source event occurred.
	EventOccurredAt time.Time

	// SourceType classifies the payment source type.
	SourceType string
	// SourceID identifies the payment source.
	SourceID string

	// PaymentStatus is the normalized payment status.
	PaymentStatus model.FinancialPaymentResultStatus
}

// ActivateOutput reports contract activation after first installment payment.
type ActivateOutput struct {
	// Result reports the use-case outcome classification.
	Result ActivateResult

	// Contract is the activated or affected contract.
	Contract model.Contract
	// FirstInstallment is the first installment after activation.
	FirstInstallment model.Installment
	// RemainingInstallments lists installments remaining after the first.
	RemainingInstallments []model.Installment

	// PaidChargeCount is the number of installment charges marked paid.
	PaidChargeCount int64
	// AppliedChargeCount is the number of installment charges applied.
	AppliedChargeCount int64

	// AddedActiveExposureAmountMinor is the active exposure amount added in minor currency units.
	AddedActiveExposureAmountMinor uint64
}
