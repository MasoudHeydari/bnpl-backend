// Package financialcorepaymentsessionuc applies inbound Financial Core payment
// session updates to BNPL payment attempts.
//
// Processing is idempotent via the inbox and keeps local attempt state aligned
// with session status reported by Financial Core.
package financialcorepaymentsessionuc
