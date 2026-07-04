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

	// ProcessingPaymentStatus indicates provider verification may have completed
	// while FC accounting is still pending.
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

const (
	// PartyCustomer is the Financial Core party_type for a Customer Reference
	// Service customer. Legacy party_type "subject" is rejected.
	PartyCustomer = "customer"

	// PartyPlatform is the Financial Core party_type for a platform cash
	// recipient. When PrepareCollectionObligation returns this type,
	// payment_recipient_id / CreateCheckoutPayment payee.party_id is empty.
	PartyPlatform = "platform"

	// PartyMerchant is the Financial Core party_type for a merchant. Merchant
	// remains the economic seller on collection/entitlement RPCs. CreateCheckoutPayment
	// payee uses it only when PrepareCollectionObligation returns
	// payment_recipient_type=merchant.
	PartyMerchant = "merchant"
)

// CreateCheckoutPaymentInput carries transport-neutral CreateCheckoutPayment
// inputs for Financial Core checkout payment v2.
//
// FinancialScope, caller-controlled source_service, request fingerprint, and
// idempotency_record_id are intentionally absent: Financial Core owns them.
//
// BNPL CreateCheckoutPayment parties:
//   - payer: PartyCustomer + trusted Customer Reference Service customer_id
//   - payee: copied from PrepareCollectionObligationResult payment_recipient_*
//     (new obligations typically PartyPlatform with empty party_id; never invent
//     a default when Prepare omitted the recipient)
//
// Service-to-service authentication (SPIFFE / mesh / mTLS) is separate from
// these party fields and must not be confused with a customer Bearer token.
type CreateCheckoutPaymentInput struct {
	// IdempotencyKey is the stable BNPL idempotency identity for this request.
	IdempotencyKey string

	// SourceType identifies the kind of business source reference.
	SourceType string
	// SourceID is the originating business source identifier.
	SourceID string
	// SourceVersion is the optional version of the source reference.
	SourceVersion *int64

	// PayerType identifies the kind of payer party (must be "customer" for BNPL).
	PayerType string
	// PayerID is the trusted Customer Reference Service customer_id.
	PayerID string
	// PayeeType is CreateCheckoutPayment payee.party_type, taken from
	// PrepareCollectionObligationResult.PaymentRecipientType.
	PayeeType string
	// PayeeID is CreateCheckoutPayment payee.party_id, taken from
	// PrepareCollectionObligationResult.PaymentRecipientID. Empty for platform.
	PayeeID string

	// AmountMinor is the payment amount in minor currency units.
	AmountMinor int64
	// Currency is the ISO currency code for AmountMinor.
	Currency string

	// ProviderCode selects the Financial Core payment provider to use.
	ProviderCode string

	// CollectionObligationID is the Financial-owned collection obligation
	// target for BNPL's initial checkout payment. Required for the collection
	// CheckoutPaymentTarget kind.
	CollectionObligationID string

	// SuccessReturnURL is where the payer is redirected after a successful payment.
	SuccessReturnURL string
	// FailureReturnURL is where the payer is redirected after a failed payment.
	FailureReturnURL string
	// CancelReturnURL is where the payer is redirected after canceling payment.
	CancelReturnURL string

	// Description is a required human-readable payment description.
	Description string
	// ExpiresAt is when the payment session expires.
	ExpiresAt time.Time
}

// CreateCheckoutPaymentResult carries transport-neutral CreateCheckoutPayment outputs.
//
// CreateCheckoutPayment is idempotent: repeating the same idempotency key for the
// same payment operation returns the same PaymentIntentID, PaymentSessionID, and
// immutable PaymentLink. There is no supported FC-hosted-URL-to-gateway-URL
// replacement lifecycle for one PaymentSession. After a network timeout where FC
// may already have created the session, recover by retrying with the original
// idempotency key — never mint a new key solely to rediscover session metadata.
type CreateCheckoutPaymentResult struct {
	InvoiceID        string
	PaymentIntentID  string
	PaymentAttemptID string
	PaymentSessionID string
	Status           PaymentStatus
	AmountMinor      int64
	Currency         string
	// PaymentLink is the immutable customer PaymentURL for this PaymentSession.
	// It is not evidence of payment success; browser redirects are not
	// authoritative payment results.
	PaymentLink            string
	ExpiresAt              time.Time
	ReusedIdempotentResult bool
}

// GetPaymentStatusInput carries transport-neutral GetPaymentStatus inputs.
type GetPaymentStatusInput struct {
	SourceType      string
	SourceID        string
	SourceVersion   *int64
	PaymentIntentID string
}

// GetPaymentStatusResult carries transport-neutral GetPaymentStatus outputs.
//
// Per the Financial Core API contract this result does not include PaymentURL /
// PaymentLink. It must not be used to obtain or refresh a payment link. Missing
// session URL after an uncertain CreateCheckoutPayment is recovered by retrying
// CreateCheckoutPayment with the original idempotency key (or by authoritative
// reconciliation paths that may optionally carry session metadata).
type GetPaymentStatusResult struct {
	PaymentIntentID string
	Status          PaymentStatus
	AmountMinor     int64
	Currency        string
	CreatedAt       time.Time
	ExpiresAt       time.Time
	CompletedAt     time.Time
}

// CollectionApplication identifies how Financial Core must apply collected cash.
type CollectionApplication string

const (
	// PreRecognitionCashCollectionApplication collects cash before receivable recognition.
	PreRecognitionCashCollectionApplication CollectionApplication = "pre_recognition_cash"

	// ExistingReceivableCollectionApplication applies cash to an already-recognized
	// merchant entitlement receivable.
	ExistingReceivableCollectionApplication CollectionApplication = "existing_receivable"
)

// PrepareCollectionObligationInput carries Financial Core collection facts.
type PrepareCollectionObligationInput struct {
	SourceType            string
	SourceID              string
	SourceVersion         *int64
	ExternalMerchantID    string
	ExternalCustomerID    string // Customer Reference Service customer_id
	Currency              string
	AmountMinor           int64
	Application           CollectionApplication
	MerchantEntitlementID string // required when Application is existing_receivable
	Description           string
	DueAt                 time.Time
}

// PrepareCollectionObligationResult identifies the durable collection obligation.
type PrepareCollectionObligationResult struct {
	CollectionObligationID string
	OutstandingAmountMinor int64
	Currency               string
	ReusedIdempotentResult bool
	// PaymentRecipientType is the CreateCheckoutPayment payee.party_type
	// returned by Financial Core. Required; callers must not invent a default.
	PaymentRecipientType string
	// PaymentRecipientID is the CreateCheckoutPayment payee.party_id. Empty when
	// PaymentRecipientType is PartyPlatform (proto optional field omitted).
	PaymentRecipientID string
}

// RecordMerchantEntitlementInput carries final economic facts for FC recognition.
type RecordMerchantEntitlementInput struct {
	SourceType                        string
	SourceID                          string
	SourceVersion                     *int64
	ExternalMerchantID                string
	ExternalCustomerID                string // Customer Reference Service customer_id
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
	ReusedIdempotentResult bool
}

// Client calls the Financial Core APIs required by checkout activation.
type Client interface {
	PrepareCollectionObligation(ctx context.Context, input PrepareCollectionObligationInput) (PrepareCollectionObligationResult, error)

	// CreateCheckoutPayment creates a checkout payment intent and session.
	//
	// BNPL payments use payer=customer (customer_id) and payee copied from
	// PrepareCollectionObligation payment_recipient_*. No customer
	// financial_delegation Bearer token is required or attached. Service
	// identity remains a separate transport concern (SPIFFE / mesh / mTLS when
	// configured).
	CreateCheckoutPayment(ctx context.Context, input CreateCheckoutPaymentInput) (CreateCheckoutPaymentResult, error)

	GetPaymentStatus(ctx context.Context, input GetPaymentStatusInput) (GetPaymentStatusResult, error)

	RecordMerchantEntitlement(ctx context.Context, input RecordMerchantEntitlementInput) (RecordMerchantEntitlementResult, error)
}
