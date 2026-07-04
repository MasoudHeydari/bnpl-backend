package otprp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core OTP repository contract.
var _ repo.OTP = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the OTP repository.
type Repo struct{}

// New returns a stateless PostgreSQL OTP repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.OTPConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.OTPTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.OTPConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByPurposeSubject(
	ctx context.Context,
	purpose model.OTPPurpose,
	subjectKey string,
) (model.OTPChallenge, error) {
	return GetByPurposeSubject(ctx, q.Conn, purpose, subjectKey)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.OTPTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByPurposeSubject(
	ctx context.Context,
	purpose model.OTPPurpose,
	subjectKey string,
) (model.OTPChallenge, error) {
	return GetByPurposeSubject(ctx, q.Tx, purpose, subjectKey)
}

func (q txQueryer) EnsureAndLock(
	ctx context.Context,
	purpose model.OTPPurpose,
	subjectKey string,
	now time.Time,
) (model.OTPChallenge, error) {
	return EnsureAndLock(
		ctx,
		q.Tx,
		purpose,
		subjectKey,
		now,
	)
}

func (q txQueryer) Issue(
	ctx context.Context,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
	verifier string,
	contextVersion string,
	expiresAt time.Time,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	return Issue(
		ctx,
		q.Tx,
		id,
		issueID,
		verifier,
		contextVersion,
		expiresAt,
		updatedAt,
	)
}

func (q txQueryer) MarkNotificationAccepted(
	ctx context.Context,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
	acceptedAt time.Time,
	requestID string,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	return MarkNotificationAccepted(
		ctx,
		q.Tx,
		id,
		issueID,
		acceptedAt,
		requestID,
		updatedAt,
	)
}

func (q txQueryer) ClearIssue(
	ctx context.Context,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	return ClearIssue(
		ctx,
		q.Tx,
		id,
		issueID,
		updatedAt,
	)
}

func (q txQueryer) RecordFailure(
	ctx context.Context,
	id model.OTPChallengeID,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	return RecordFailure(ctx, q.Tx, id, updatedAt)
}

func (q txQueryer) MarkVerified(
	ctx context.Context,
	id model.OTPChallengeID,
	contextVersion string,
	verifiedAt time.Time,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	return MarkVerified(
		ctx,
		q.Tx,
		id,
		contextVersion,
		verifiedAt,
		updatedAt,
	)
}

func (q txQueryer) ConsumeRateLimit(
	ctx context.Context,
	scope model.OTPRateLimitScope,
	scopeKey string,
	window time.Duration,
	maxSends int,
	now time.Time,
) error {
	return ConsumeRateLimit(
		ctx,
		q.Tx,
		scope,
		scopeKey,
		window,
		maxSends,
		now,
	)
}
