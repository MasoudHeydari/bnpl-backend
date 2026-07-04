// Package paymentattemptexpirationuc closes the local CreateCheckoutPayment
// window for installment repayment PaymentAttempts after the immutable session
// TTL and post-lease safety interval.
//
// Expiration is a BNPL orchestration outcome only. It never calls Financial
// Core, never invents payment failure or cancellation, and leaves
// financial_outcome_resolved false so late authoritative FC results remain
// applicable. Scope is installment_repayment only; checkout_first_installment
// keeps its existing contract-expiration path.
package paymentattemptexpirationuc
