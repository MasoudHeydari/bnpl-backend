package event

// OTP issuance and verification checkpoints.
var (
	OTPIssueRequested = define(
		"otp.issue.requested",
		"otp issuance requested",
	)
	OTPIssueRateLimited = define(
		"otp.issue.rate_limited",
		"otp issuance rate limited",
	)
	OTPChallengeReused = define(
		"otp.challenge.reused",
		"existing otp challenge reused",
	)
	OTPChallengeIssued = define(
		"otp.challenge.issued",
		"otp challenge issued and committed",
	)
	OTPNotificationSubmitStarted = define(
		"otp.notification.submit_started",
		"otp notification submission started",
	)
	OTPProviderAccepted = define(
		"otp.notification.provider_accepted",
		"otp notification provider accepted request",
	)
	OTPProviderRejected = define(
		"otp.notification.provider_rejected",
		"otp notification provider rejected request",
	)
	OTPProviderTimedOut = define(
		"otp.notification.provider_timed_out",
		"otp notification provider timed out",
	)
	OTPVerificationAttempted = define(
		"otp.verification.attempted",
		"otp verification attempted",
	)
	OTPVerificationSucceeded = define(
		"otp.verification.succeeded",
		"otp verification succeeded",
	)
	OTPVerificationFailed = define(
		"otp.verification.failed",
		"otp verification failed",
	)
	OTPChallengeExpired = define(
		"otp.challenge.expired",
		"otp challenge expired",
	)
	OTPChallengeLocked = define(
		"otp.challenge.locked",
		"otp challenge locked",
	)
)
