// Package checkoutpayment defines the business-facing Financial Core port for
// creating checkout payment sessions.
//
// Financial Core owns monetary payment outcomes. BNPL correlates requests with
// stable idempotency identity and never invents payment success from local state.
// This package must stay transport-neutral: no protobuf or gRPC types belong here.
package checkoutpayment
