// Package financialcorepaymentreconciliationuc applies inbound Financial Core
// installment-payment reconciliation results to BNPL local state.
//
// Processing is idempotent via the inbox and must not invent monetary outcomes
// beyond what Financial Core reports.
//
// Operational note: FC currently has no active consumer for
// bnpl.installment_payment_reconciliation_requested.v1, so BNPL no longer emits
// that request as a recovery mechanism. Submitted-payment recovery uses
// GetPaymentStatus (see paymentattemptreconciliationuc). This inbound package
// remains for any future/authorized reconciliation result events and must not
// be treated as proof that Kafka request/response reconciliation is live.
package financialcorepaymentreconciliationuc
