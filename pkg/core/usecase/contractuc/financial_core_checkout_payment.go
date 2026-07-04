package contractuc

// CheckoutPaymentOutput returns Financial Core checkout payment details to callers.
type CheckoutPaymentOutput struct {
	// InvoiceID identifies the Financial Core invoice.
	InvoiceID string
	// PaymentIntentID identifies the Financial Core payment intent.
	PaymentIntentID string
	// PaymentAttemptID identifies the payment attempt in Financial Core or BNPL context.
	PaymentAttemptID string
	// PaymentSessionID identifies the Financial Core payment session.
	PaymentSessionID string

	// PaymentLink is the hosted payment link for the customer.
	PaymentLink string
	// ExpiresAt is when the offer, session, or limit expires.
	ExpiresAt string

	// ReusedIdempotentResult indicates the output was produced by an idempotent replay.
	ReusedIdempotentResult bool
}

const (
	financialSourceService = "bnpl-core"
	financialSourceType    = "bnpl_contract"

	// BNPL purchase CreateCheckoutPayment parties (Financial Core contract):
	// payer = trusted Customer Reference Service customer; payee = platform with
	// an intentionally empty party_id. Merchant remains the economic seller via
	// PrepareCollectionObligation / RecordMerchantEntitlement — not the checkout
	// payment payee.
	financialPayerType = "customer"
	financialPayeeType = "platform"

	// financialPlatformPayeeID is required to be empty by Financial Core for
	// platform checkout payees. Do not substitute MerchantID or invent a UUID.
	financialPlatformPayeeID = ""
)
