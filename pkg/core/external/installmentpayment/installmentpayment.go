// Package installmentpayment defines the business-facing Financial Core port for
// creating installment repayment payment sessions.
//
// Financial Core owns monetary payment outcomes. BNPL correlates requests with
// stable idempotency identity and never invents repayment success from local state.
// This package must stay transport-neutral: no protobuf or gRPC types belong here.
package installmentpayment
