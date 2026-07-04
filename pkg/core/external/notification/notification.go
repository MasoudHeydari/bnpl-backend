// Package notification defines the core abstraction for durable notification
// submission.
//
// BNPL owns OTP generation, verification, expiry, and rate limits. The
// Notification Service owns durable acceptance of a submission request; it
// does not verify OTPs and does not imply provider delivery.
package notification
