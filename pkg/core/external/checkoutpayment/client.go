package checkoutpayment

import (
	"context"
	"time"
)

// PaymentStatus identifies the Financial Core checkout payment status.
type PaymentStatus string

const (
	// PendingPaymentStatus indicates the payment is not yet terminal.
	PendingPaymentStatus PaymentStatus = "pending"

	// RequiresActionPaymentStatus indicates the customer must complete an action.
	RequiresActionPaymentStatus PaymentStatus = "requires_action"

	// ProcessingPaymentStatus indicates provider verification may have completed while FC accounting is still pending.
	ProcessingPaymentStatus PaymentStatus = "processing"

	// SucceededPaymentStatus indicates the payment succeeded.
	SucceededPaymentStatus PaymentStatus = "succeeded"

	// FailedPaymentStatus indicates the payment terminally failed.
	FailedPaymentStatus PaymentStatus = "failed"

	// CanceledPaymentStatus indicates the payment was canceled.
	CanceledPaymentStatus PaymentStatus = "canceled"

	// ExpiredPaymentStatus indicates the payment session expired before completion.
	ExpiredPaymentStatus PaymentStatus = "expired"
)

// CreateCheckoutPaymentInput carries transport-neutral CreateCheckoutPayment inputs.
type CreateCheckoutPaymentInput struct {
	// IdempotencyKey is the stable Financial Core idempotency identity for this request.
	IdempotencyKey string
	// IdempotencyRecordID is BNPL's local idempotency record identifier for correlation.
	IdempotencyRecordID string
	// RequestFingerprint is a hash of request material used to detect payload conflicts.
	RequestFingerprint string

	// TenantID identifies the Financial Core tenant for the payment.
	TenantID string
	// ContextID identifies the Financial Core request context for the payment.
	ContextID string

	// SourceService identifies the originating service that owns the source reference.
	SourceService string
	// SourceType identifies the kind of source reference (for example a BNPL decision).
	SourceType string
	// SourceID is the originating source reference identifier.
	SourceID string
	// SourceVersion is the contract version of the source reference.
	SourceVersion int64

	// PayerType identifies the kind of payer party.
	PayerType string
	// PayerID identifies the payer party.
	PayerID string
	// PayeeType identifies the kind of payee party.
	PayeeType string
	// PayeeID identifies the payee party.
	PayeeID string

	// AmountMinor is the payment amount in minor currency units.
	AmountMinor int64
	// Currency is the ISO currency code for AmountMinor.
	Currency string

	// ProviderCode selects the Financial Core payment provider to use.
	ProviderCode string

	// CollectionObligationID identifies the prepared Financial Core collection obligation.
	CollectionObligationID string

	// SuccessReturnURL is where the payer is redirected after a successful payment.
	SuccessReturnURL string
	// FailureReturnURL is where the payer is redirected after a failed payment.
	FailureReturnURL string
	// CancelReturnURL is where the payer is redirected after canceling payment.
	CancelReturnURL string

	// Description is an optional human-readable payment description.
	Description string
	// ExpiresAt is when the payment session expires.
	ExpiresAt time.Time
}

// CreateCheckoutPaymentResult carries transport-neutral CreateCheckoutPayment outputs.
type CreateCheckoutPaymentResult struct {
	// InvoiceID is the Financial Core invoice identifier when returned.
	InvoiceID string
	// PaymentIntentID is the Financial Core payment intent identifier.
	PaymentIntentID string
	// PaymentAttemptID is the Financial Core payment attempt identifier.
	PaymentAttemptID string
	// PaymentSessionID is the Financial Core payment session identifier.
	PaymentSessionID string

	// Status is the current Financial Core checkout payment status.
	Status PaymentStatus

	// AmountMinor is the payment amount in minor currency units.
	AmountMinor int64
	// Currency is the ISO currency code for AmountMinor.
	Currency string

	// PaymentLink is the URL the payer uses to complete payment.
	PaymentLink string
	// ExpiresAt is when the payment session expires.
	ExpiresAt time.Time

	// ReusedIdempotentResult reports that Financial Core returned a prior idempotent result.
	ReusedIdempotentResult bool
}

// GetPaymentStatusInput carries transport-neutral GetPaymentStatus inputs.
type GetPaymentStatusInput struct {
	// TenantID identifies the Financial Core tenant for the lookup.
	TenantID string
	// ContextID identifies the Financial Core request context for the lookup.
	ContextID string

	// SourceService identifies the originating service that owns the source reference.
	SourceService string
	// SourceType identifies the kind of source reference.
	SourceType string
	// SourceID is the originating source reference identifier.
	SourceID string
	// SourceVersion is the contract version of the source reference.
	SourceVersion int64

	// PaymentIntentID is the Financial Core payment intent to query.
	PaymentIntentID string
}

// GetPaymentStatusResult carries transport-neutral GetPaymentStatus outputs.
type GetPaymentStatusResult struct {
	// PaymentIntentID is the Financial Core payment intent identifier.
	PaymentIntentID string
	// Status is the current Financial Core checkout payment status.
	Status PaymentStatus

	// AmountMinor is the payment amount in minor currency units.
	AmountMinor int64
	// Currency is the ISO currency code for AmountMinor.
	Currency string

	// CreatedAt is when Financial Core created the payment intent.
	CreatedAt time.Time
	// ExpiresAt is when the payment session expires.
	ExpiresAt time.Time
	// CompletedAt is when the payment reached a terminal status, if known.
	CompletedAt time.Time
}

// CollectionApplication identifies how Financial Core must apply collected cash.
type CollectionApplication string

const (
	// PreRecognitionCashCollectionApplication is used when cash is collected before receivable recognition.
	PreRecognitionCashCollectionApplication CollectionApplication = "pre_recognition_cash"

	// ExistingReceivableCollectionApplication applies cash to an already-recognized receivable.
	ExistingReceivableCollectionApplication CollectionApplication = "existing_receivable"
)

// PrepareCollectionObligationInput carries the generic Financial Core collection facts needed before checkout creation.
type PrepareCollectionObligationInput struct {
	SourceType        string
	SourceID          string
	SourceVersion     int64
	MerchantID        string
	CustomerSubjectID string
	Currency          string
	AmountMinor       int64
	Application       CollectionApplication
	ReceivableID      string
	Description       string
	DueAt             time.Time
}

// PrepareCollectionObligationResult identifies the durable Financial Core collection obligation.
type PrepareCollectionObligationResult struct {
	CollectionObligationID string
	OutstandingAmountMinor int64
	Currency               string
	ReusedIdempotentResult bool
}

// RecordMerchantEntitlementInput carries the final economic facts Financial Core must recognize as part of BNPL contract activation.
type RecordMerchantEntitlementInput struct {
	SourceType                        string
	SourceID                          string
	SourceVersion                     int64
	MerchantID                        string
	CustomerSubjectID                 string
	Currency                          string
	CollectedCashAmountMinor          int64
	ReceivableAmountMinor             int64
	MerchantPayableAmountMinor        int64
	CollectedCashPaymentIntentID      string
	MerchantTransactionFeeAmountMinor int64
}

// RecordMerchantEntitlementResult identifies the durable FC merchant entitlement.
type RecordMerchantEntitlementResult struct {
	MerchantEntitlementID  string
	ReceivableID           string
	ReusedIdempotentResult bool
}

// Client calls the Financial Core APIs required by checkout activation.
type Client interface {
	// PrepareCollectionObligation prepares the pre-recognition collection instrument used by checkout.
	PrepareCollectionObligation(ctx context.Context, input PrepareCollectionObligationInput) (PrepareCollectionObligationResult, error)

	// CreateCheckoutPayment creates a checkout payment intent and session in Financial Core.
	CreateCheckoutPayment(
		ctx context.Context,
		input CreateCheckoutPaymentInput,
	) (CreateCheckoutPaymentResult, error)

	// GetPaymentStatus returns the current Financial Core status for a checkout payment intent.
	GetPaymentStatus(
		ctx context.Context,
		input GetPaymentStatusInput,
	) (GetPaymentStatusResult, error)

	// RecordMerchantEntitlement synchronously records the Financial Core economic recognition required by contract activation.
	RecordMerchantEntitlement(ctx context.Context, input RecordMerchantEntitlementInput) (RecordMerchantEntitlementResult, error)
}
