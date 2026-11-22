// Package paymentattemptreconciliationuc recovers uncertain payment-attempt
// outcomes through Financial Core GetPaymentStatus.
//
// Background jobs drive this package. It does not emit
// bnpl.installment_payment_reconciliation_requested.v1: FC has no active
// consumer for that topic. Session-creation rediscovery remains outside this
// package (same-key CreateCheckoutPayment replay). Operational review
// resolution is also hosted here so documented FC findings can resume or
// safely terminate workflows without inventing monetary truth.
package paymentattemptreconciliationuc
