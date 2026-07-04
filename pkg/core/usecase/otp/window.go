package otp

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// challengeUsableAt reports whether an unused, unexpired OTP challenge exists
// for expectedContextVersion at now.
func challengeUsableAt(challenge model.OTPChallenge, expectedContextVersion string, now time.Time) bool {
	if challenge.ContextVersion != expectedContextVersion {
		return false
	}

	return challenge.IssueID != nil &&
		!challenge.IssueID.IsZero() &&
		challenge.Verifier != nil &&
		challenge.ExpiresAt != nil &&
		now.Before(*challenge.ExpiresAt)
}

// issuanceWindowActiveAt reports whether the durable original issuance window
// still blocks a new OTP at now. Eligibility begins at the exact deadline
// (!now.Before(next) ⇒ eligible).
func issuanceWindowActiveAt(challenge model.OTPChallenge, now time.Time) bool {
	if challenge.NextIssueAt == nil {
		return false
	}

	return now.Before(challenge.NextIssueAt.UTC())
}

// nextIssueAtCopy returns a copy of the durable next-issue deadline when set.
func nextIssueAtCopy(challenge model.OTPChallenge) *time.Time {
	if challenge.NextIssueAt == nil {
		return nil
	}

	at := challenge.NextIssueAt.UTC()

	return &at
}

// activeExpiryCopy returns a copy of expires_at when the challenge is still
// usable for expectedContextVersion at now.
func activeExpiryCopy(challenge model.OTPChallenge, expectedContextVersion string, now time.Time) *time.Time {
	if !challengeUsableAt(challenge, expectedContextVersion, now) {
		return nil
	}

	at := challenge.ExpiresAt.UTC()

	return &at
}

// verificationLockedAt reports whether failed-verification attempts exhaust
// the configured budget for the current challenge.
func verificationLockedAt(challenge model.OTPChallenge, maxAttempts int) bool {
	return challenge.Verifier != nil && challenge.AttemptCount >= maxAttempts
}

// secondsUntilNextIssue returns a non-negative whole-second wait, or nil when
// issuance is already eligible by timer.
func secondsUntilNextIssue(challenge model.OTPChallenge, now time.Time) *int {
	if !issuanceWindowActiveAt(challenge, now) {
		return nil
	}

	wait := int(challenge.NextIssueAt.UTC().Sub(now.UTC()).Seconds())
	if wait < 1 {
		wait = 1
	}

	return &wait
}
