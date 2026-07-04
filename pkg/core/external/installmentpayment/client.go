package installmentpayment

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreateInstallmentPaymentInput carries transport-neutral installment repayment inputs.
type CreateInstallmentPaymentInput struct {
	// TenantID identifies the Financial Core tenant for the payment.
	TenantID string
	// ContextID identifies the Financial Core request context for the payment.
	ContextID string

	// ContractID is the BNPL contract being repaid.
	ContractID model.ContractID
	// InstallmentID is the BNPL installment being repaid.
	InstallmentID model.InstallmentID

	// CustomerID is the internal BNPL customer primary key for the repayment.
	CustomerID model.CustomerID

	// AmountMinor is the repayment amount in minor currency units.
	AmountMinor int64
	// Currency is the ISO currency code for AmountMinor.
	Currency string
	// Channel is the repayment channel selected for the payment session.
	Channel model.PaymentChannel

	// IdempotencyKey is the stable Financial Core idempotency identity for this request.
	IdempotencyKey string
	// RequestFingerprint is a hash of request material used to detect payload conflicts.
	RequestFingerprint string
	// IdempotencyRecordID is BNPL's local idempotency record identifier for correlation.
	IdempotencyRecordID string

	// ReturnURL is an optional URL where the payer is redirected after payment.
	ReturnURL *string
}

// CreateInstallmentPaymentResult carries transport-neutral installment repayment outputs.
type CreateInstallmentPaymentResult struct {
	// FinancialPaymentIntentID is the Financial Core payment intent identifier.
	FinancialPaymentIntentID string
	// FinancialPaymentSessionID is the Financial Core payment session identifier.
	FinancialPaymentSessionID string
	// PaymentURL is the URL the payer uses to complete repayment, when available.
	PaymentURL *string
	// ExpiresAt is when the payment session expires, if known.
	ExpiresAt *time.Time
	// ReusedIdempotentResult reports that Financial Core returned a prior idempotent result.
	ReusedIdempotentResult bool
}

// Client creates Financial Core installment repayment sessions.
type Client interface {
	// CreateInstallmentPayment creates an installment repayment session in Financial Core.
	CreateInstallmentPayment(
		ctx context.Context,
		input CreateInstallmentPaymentInput,
	) (CreateInstallmentPaymentResult, error)
}
