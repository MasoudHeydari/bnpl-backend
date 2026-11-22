package model

import "time"

// InstallmentRepaymentPaymentSessionTTL is the immutable CreateCheckoutPayment
// expires_at offset from PaymentAttempt.CreatedAt for installment repayment.
//
// Retries of the same PaymentAttempt must reuse CreatedAt + TTL. Do not load
// this duration from mutable runtime configuration in a way that would change
// expires_at for an in-flight attempt while reusing its FC idempotency key.
const InstallmentRepaymentPaymentSessionTTL = 15 * time.Minute

// MaxFinancialSessionCreateAttempts is how many customer-triggered synchronous
// CreateCheckoutPayment calls one PaymentAttempt / idempotency key may start
// (1 initial + 2 retries). A fourth request must not call Financial Core.
// Exhausting this budget cancels the attempt locally with
// SessionCreationRetryExhaustedFailureCode and financial_outcome_resolved=false.
// It is not authoritative FC failure/cancellation and does not invent a paid or
// unpaid monetary outcome. Abandoned attempts that stop earlier are expired by
// TTL cleanup instead. Independent of submitted-payment outcome reconciliation.
const MaxFinancialSessionCreateAttempts = 3

// SessionCreationRetryExhaustedFailureCode marks a PaymentAttempt that was
// locally canceled because three synchronous CreateCheckoutPayment invocations
// failed or returned an unusable session. Distinct from authoritative FC
// cancellation (financial_outcome_resolved=true).
const SessionCreationRetryExhaustedFailureCode = "session_creation_retry_exhausted"

// FinancialSessionCreateLeaseDuration is how long one reservation owns the
// right to start CreateCheckoutPayment.
//
// Write RPCs (CreateCheckoutPayment, RecordMerchantEntitlement,
// PrepareCollectionObligation) use a 100s call deadline in the Financial Core
// gRPC adapter. The lease must outlive that deadline so a late response cannot
// be claimed by a second worker while the first call is still in flight.
// Lease expiry is not payment failure and does not discard a later verified
// Financial Core response.
const FinancialSessionCreateLeaseDuration = 120 * time.Second

// FinancialSessionCreateExpirationSafetyInterval is added after
// FinancialSessionCreateLeaseUntil before local session-creation expiration
// may run. It covers a late CreateCheckoutPayment response after lease expiry
// without treating the safety window as proof that Financial Core did nothing.
const FinancialSessionCreateExpirationSafetyInterval = 60 * time.Second
