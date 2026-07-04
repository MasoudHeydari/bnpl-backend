// Package otp implements a reusable one-time-code issuance and verification
// use case.
//
// It owns challenge generation, durable issuance windows, single-use
// consumption, verification lockouts, per-subject and cross-subject send
// budgets, Notification submission coordination, and privacy-safe availability
// projection. Application-specific eligibility (for example hosted checkout
// confirm rules) is supplied through IssuanceAuthorizer and caller-provided
// availability hints — this package does not import checkoutsessionuc or
// contractuc.
package otp
