// Package financialcorepaymentreconciliationuc applies inbound Financial Core
// installment-payment reconciliation results to BNPL local state.
//
// Processing is idempotent via the inbox and must not invent monetary outcomes
// beyond what Financial Core reports.
package financialcorepaymentreconciliationuc
