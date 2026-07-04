// Package otprp implements the OTP challenge repository with PostgreSQL.
//
// Challenges live in otp_challenges (one row per purpose/subject). Cross-
// subject send budgets live in otp_rate_limits. Issue, consume, and
// notification CAS updates are transaction-scoped so callers can compose them
// with application eligibility locks in the same Tx.
package otprp
