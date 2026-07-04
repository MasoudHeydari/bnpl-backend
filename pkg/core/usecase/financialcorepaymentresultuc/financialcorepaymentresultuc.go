// Package financialcorepaymentresultuc applies inbound Financial Core payment
// results to BNPL contracts, installments, and payment attempts.
//
// Processing is idempotent via the inbox. Financial Core remains the authority
// for monetary payment outcomes; BNPL updates local lifecycle and exposure state.
package financialcorepaymentresultuc
