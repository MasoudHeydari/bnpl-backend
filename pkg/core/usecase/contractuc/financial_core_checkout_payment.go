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

	financialPayerType = "customer"
	financialPayeeType = "merchant"
)
