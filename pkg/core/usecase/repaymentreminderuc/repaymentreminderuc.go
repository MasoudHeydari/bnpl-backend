// Package repaymentreminderuc dispatches repayment reminder SMS requests through
// the transactional outbox.
//
// OTP notifications remain on the existing Notification gRPC path. This package
// must never call Notification Service over gRPC.
package repaymentreminderuc
