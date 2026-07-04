package otprp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const otpChallengeColumns = `
	id,
	purpose,
	subject_key,
	context_version,
	issue_id,
	verifier,
	expires_at,
	next_issue_at,
	notification_accepted_at,
	verified_at,
	attempt_count,
	notification_request_id,
	send_count,
	created_at,
	updated_at
`

// GetByPurposeSubject returns the challenge row for purpose and subjectKey.
func GetByPurposeSubject[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	purpose model.OTPPurpose,
	subjectKey string,
) (model.OTPChallenge, error) {
	const op = "otprp.GetByPurposeSubject"

	if !purpose.IsValid() {
		return model.OTPChallenge{}, cerr.InternalError(
			fmt.Errorf("%s: unsupported purpose %q", op, purpose),
		)
	}

	if subjectKey == "" {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: subject key is required", op))
	}

	const query = `
SELECT` + otpChallengeColumns + `
FROM otp_challenges
WHERE purpose = $1 AND subject_key = $2
`

	return postgres.QueryOne[model.OTPChallenge](
		ctx,
		q,
		"get otp challenge by purpose subject",
		query,
		purpose,
		subjectKey,
	)
}

// EnsureAndLock inserts a blank challenge row when missing and locks it.
func EnsureAndLock[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	purpose model.OTPPurpose,
	subjectKey string,
	now time.Time,
) (model.OTPChallenge, error) {
	const op = "otprp.EnsureAndLock"

	if !purpose.IsValid() {
		return model.OTPChallenge{}, cerr.InternalError(
			fmt.Errorf("%s: unsupported purpose %q", op, purpose),
		)
	}

	if subjectKey == "" {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: subject key is required", op))
	}

	if err := postgres.RequiredTime("now", now); err != nil {
		return model.OTPChallenge{}, err
	}

	const ensureQuery = `
INSERT INTO otp_challenges (
	purpose,
	subject_key,
	context_version,
	created_at,
	updated_at
)
VALUES ($1, $2, '', $3, $3)
ON CONFLICT (purpose, subject_key) DO NOTHING
`

	if _, err := q.Exec(
		ctx,
		ensureQuery,
		purpose,
		subjectKey,
		now,
	); err != nil {
		return model.OTPChallenge{}, err
	}

	const lockQuery = `
SELECT` + otpChallengeColumns + `
FROM otp_challenges
WHERE purpose = $1 AND subject_key = $2
FOR UPDATE
`

	return postgres.QueryOne[model.OTPChallenge](
		ctx,
		q,
		"lock otp challenge",
		lockQuery,
		purpose,
		subjectKey,
	)
}

// Issue stores a newly issued OTP challenge.
func Issue[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
	verifier string,
	contextVersion string,
	expiresAt time.Time,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	const op = "otprp.Issue"

	if id.IsZero() {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: challenge id is required", op))
	}

	if err := issueID.Validate(); err != nil {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: %w", op, err))
	}

	if verifier == "" {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: otp verifier is required", op))
	}

	if contextVersion == "" {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: context version is required", op))
	}

	if err := postgres.RequiredTime("expires_at", expiresAt); err != nil {
		return model.OTPChallenge{}, err
	}

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.OTPChallenge{}, err
	}

	// Compare-and-swap on next_issue_at so concurrent issuers at the deadline
	// produce at most one new logical OTP.
	const query = `
UPDATE otp_challenges
SET issue_id = $2,
	verifier = $3,
	context_version = $4,
	expires_at = $5,
	next_issue_at = $5,
	notification_accepted_at = NULL,
	verified_at = NULL,
	notification_request_id = NULL,
	attempt_count = 0,
	send_count = send_count + 1,
	updated_at = $6
WHERE id = $1
  AND (next_issue_at IS NULL OR next_issue_at <= $6)
RETURNING` + otpChallengeColumns

	return postgres.QueryOne[model.OTPChallenge](
		ctx,
		q,
		"issue otp challenge",
		query,
		id,
		issueID.String(),
		verifier,
		contextVersion,
		expiresAt,
		updatedAt,
	)
}

// MarkNotificationAccepted records durable Notification acceptance for issueID.
func MarkNotificationAccepted[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
	acceptedAt time.Time,
	requestID string,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	const op = "otprp.MarkNotificationAccepted"

	if id.IsZero() {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: challenge id is required", op))
	}

	if err := issueID.Validate(); err != nil {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: %w", op, err))
	}

	if requestID == "" {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: notification request id is required", op))
	}

	if err := postgres.RequiredTime("notification_accepted_at", acceptedAt); err != nil {
		return model.OTPChallenge{}, err
	}

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.OTPChallenge{}, err
	}

	const query = `
UPDATE otp_challenges
SET notification_accepted_at = $3,
	notification_request_id = $4,
	updated_at = $5
WHERE id = $1
  AND issue_id = $2
  AND verifier IS NOT NULL
RETURNING` + otpChallengeColumns

	return postgres.QueryOne[model.OTPChallenge](
		ctx,
		q,
		"mark otp notification accepted",
		query,
		id,
		issueID.String(),
		acceptedAt,
		requestID,
		updatedAt,
	)
}

// ClearIssue discards the OTP issuance identified by issueID.
func ClearIssue[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	const op = "otprp.ClearIssue"

	if id.IsZero() {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: challenge id is required", op))
	}

	if err := issueID.Validate(); err != nil {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: %w", op, err))
	}

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.OTPChallenge{}, err
	}

	const query = `
UPDATE otp_challenges
SET issue_id = NULL,
	verifier = NULL,
	expires_at = NULL,
	next_issue_at = NULL,
	notification_accepted_at = NULL,
	notification_request_id = NULL,
	attempt_count = 0,
	updated_at = $3
WHERE id = $1
  AND issue_id = $2
  AND verified_at IS NULL
RETURNING` + otpChallengeColumns

	return postgres.QueryOne[model.OTPChallenge](
		ctx,
		q,
		"clear otp issue",
		query,
		id,
		issueID.String(),
		updatedAt,
	)
}

// RecordFailure atomically counts one failed verification attempt.
func RecordFailure[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.OTPChallengeID,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	const op = "otprp.RecordFailure"

	if id.IsZero() {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: challenge id is required", op))
	}

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.OTPChallenge{}, err
	}

	const query = `
UPDATE otp_challenges
SET attempt_count = attempt_count + 1,
	updated_at = $2
WHERE id = $1
  AND verifier IS NOT NULL
RETURNING` + otpChallengeColumns

	return postgres.QueryOne[model.OTPChallenge](
		ctx,
		q,
		"record otp failure",
		query,
		id,
		updatedAt,
	)
}

// MarkVerified records proof of possession and discards the verifier.
func MarkVerified[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.OTPChallengeID,
	contextVersion string,
	verifiedAt time.Time,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	const op = "otprp.MarkVerified"

	if id.IsZero() {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: challenge id is required", op))
	}

	if contextVersion == "" {
		return model.OTPChallenge{}, cerr.InternalError(fmt.Errorf("%s: context version is required", op))
	}

	if err := postgres.RequiredTime("verified_at", verifiedAt); err != nil {
		return model.OTPChallenge{}, err
	}

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.OTPChallenge{}, err
	}

	const query = `
UPDATE otp_challenges
SET verified_at = $2,
	issue_id = NULL,
	verifier = NULL,
	expires_at = NULL,
	updated_at = $3
WHERE id = $1
  AND verifier IS NOT NULL
  AND context_version = $4
RETURNING` + otpChallengeColumns

	return postgres.QueryOne[model.OTPChallenge](
		ctx,
		q,
		"mark otp verified",
		query,
		id,
		verifiedAt,
		updatedAt,
		contextVersion,
	)
}

// ConsumeRateLimit atomically consumes one send from the fixed-window bucket.
func ConsumeRateLimit[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	scope model.OTPRateLimitScope,
	scopeKey string,
	window time.Duration,
	maxSends int,
	now time.Time,
) error {
	const op = "otprp.ConsumeRateLimit"

	if !scope.IsValid() {
		return cerr.InternalError(fmt.Errorf("%s: unsupported rate limit scope %q", op, scope))
	}

	if scopeKey == "" {
		return cerr.InternalError(fmt.Errorf("%s: scope key is required", op))
	}

	if window <= 0 {
		return cerr.InternalError(fmt.Errorf("%s: positive window is required", op))
	}

	if maxSends <= 0 {
		return cerr.InternalError(fmt.Errorf("%s: positive max sends is required", op))
	}

	if err := postgres.RequiredTime("now", now); err != nil {
		return err
	}

	const ensureQuery = `
INSERT INTO otp_rate_limits (
	scope,
	scope_key,
	window_started_at,
	send_count,
	updated_at
)
VALUES ($1, $2, $3, 0, $3)
ON CONFLICT (scope, scope_key) DO NOTHING
`

	if _, err := q.Exec(
		ctx,
		ensureQuery,
		scope,
		scopeKey,
		now,
	); err != nil {
		return err
	}

	type rateLimitRow struct {
		Scope           model.OTPRateLimitScope `db:"scope"`
		ScopeKey        string                  `db:"scope_key"`
		WindowStartedAt time.Time               `db:"window_started_at"`
		SendCount       int                     `db:"send_count"`
		UpdatedAt       time.Time               `db:"updated_at"`
	}

	const lockQuery = `
SELECT scope, scope_key, window_started_at, send_count, updated_at
FROM otp_rate_limits
WHERE scope = $1 AND scope_key = $2
FOR UPDATE
`

	existing, err := postgres.QueryOne[rateLimitRow](
		ctx,
		q,
		"lock otp rate limit",
		lockQuery,
		scope,
		scopeKey,
	)
	if err != nil {
		return err
	}

	windowEnded := !existing.WindowStartedAt.Add(window).After(now)
	nextCount := existing.SendCount + 1
	windowStartedAt := existing.WindowStartedAt

	if windowEnded {
		nextCount = 1
		windowStartedAt = now
	}

	if nextCount > maxSends {
		return cerr.NewPublic(
			cerr.CodeTooManyRequests,
			"error.otp_rate_limited",
			fmt.Errorf("%s: rate limit exceeded for scope %q", op, scope),
			cerr.Details{Fields: []string{}, Violations: []cerr.Violation{}},
		)
	}

	const updateQuery = `
UPDATE otp_rate_limits
SET
	window_started_at = $3,
	send_count = $4,
	updated_at = $5
WHERE scope = $1 AND scope_key = $2
`

	if _, err := q.Exec(
		ctx,
		updateQuery,
		scope,
		scopeKey,
		windowStartedAt,
		nextCount,
		now,
	); err != nil {
		return err
	}

	return nil
}
