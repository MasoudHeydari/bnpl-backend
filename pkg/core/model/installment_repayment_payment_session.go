package model

import "time"

// InstallmentRepaymentPaymentSessionTTL is the immutable CreateCheckoutPayment
// expires_at offset from PaymentAttempt.CreatedAt for installment repayment.
//
// Retries of the same PaymentAttempt must reuse CreatedAt + TTL. Do not load
// this duration from mutable runtime configuration in a way that would change
// expires_at for an in-flight attempt while reusing its FC idempotency key.
const InstallmentRepaymentPaymentSessionTTL = 15 * time.Minute
