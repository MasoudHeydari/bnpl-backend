package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// OTPQueryer defines read-only OTP challenge operations.
type OTPQueryer interface {
	// GetByPurposeSubject returns the challenge row for purpose and subjectKey.
	GetByPurposeSubject(
		ctx context.Context,
		purpose model.OTPPurpose,
		subjectKey string,
	) (model.OTPChallenge, error)
}

// OTPConnQueryer exposes OTP operations on a connection.
type OTPConnQueryer interface {
	// OTPQueryer provides shared OTP query operations.
	OTPQueryer
}

// OTPTxQueryer exposes transaction-scoped OTP challenge and rate-limit operations.
type OTPTxQueryer interface {
	// OTPQueryer provides shared OTP query operations.
	OTPQueryer

	// EnsureAndLock inserts a blank challenge row for purpose/subjectKey when
	// missing and returns the row locked FOR UPDATE.
	EnsureAndLock(
		ctx context.Context,
		purpose model.OTPPurpose,
		subjectKey string,
		now time.Time,
	) (model.OTPChallenge, error)

	// Issue stores a newly issued OTP challenge under a fresh opaque issueID,
	// binds it to contextVersion, sets expires_at and next_issue_at to the same
	// issuance-window deadline, and increments send_count. It resets challenge
	// fields (accepted/verified/request id cleared, attempt count zeroed) so
	// each issue starts a clean attempt budget.
	Issue(
		ctx context.Context,
		id model.OTPChallengeID,
		issueID model.OTPIssueID,
		verifier string,
		contextVersion string,
		expiresAt time.Time,
		updatedAt time.Time,
	) (model.OTPChallenge, error)

	// MarkNotificationAccepted records that Notification durably accepted
	// submission of the OTP issuance identified by issueID. Stale issue IDs
	// must not update a newer challenge.
	MarkNotificationAccepted(
		ctx context.Context,
		id model.OTPChallengeID,
		issueID model.OTPIssueID,
		acceptedAt time.Time,
		requestID string,
		updatedAt time.Time,
	) (model.OTPChallenge, error)

	// ClearIssue discards the OTP issuance identified by issueID after a
	// definite Notification rejection. Stale failure handlers must not clear a
	// newer issuance.
	ClearIssue(
		ctx context.Context,
		id model.OTPChallengeID,
		issueID model.OTPIssueID,
		updatedAt time.Time,
	) (model.OTPChallenge, error)

	// RecordFailure atomically increments the failed-attempt counter of the
	// issued OTP.
	RecordFailure(
		ctx context.Context,
		id model.OTPChallengeID,
		updatedAt time.Time,
	) (model.OTPChallenge, error)

	// MarkVerified records proof of possession and clears the verifier and
	// expires_at so the code cannot be replayed. next_issue_at is preserved so
	// the original issuance window still gates the next SMS. The update refuses
	// a verifier bound to a different contextVersion.
	MarkVerified(
		ctx context.Context,
		id model.OTPChallengeID,
		contextVersion string,
		verifiedAt time.Time,
		updatedAt time.Time,
	) (model.OTPChallenge, error)

	// ConsumeRateLimit consumes one cross-subject OTP send from the durable
	// fixed-window bucket identified by scope and scopeKey. It must run in the
	// same transaction as Issue. Exceeding the budget returns
	// cerr.CodeTooManyRequests without mutating the counter past the max.
	ConsumeRateLimit(
		ctx context.Context,
		scope model.OTPRateLimitScope,
		scopeKey string,
		window time.Duration,
		maxSends int,
		now time.Time,
	) error
}

// OTP persists reusable OTP challenges and cross-subject rate-limit budgets.
type OTP interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) OTPConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) OTPTxQueryer
}
