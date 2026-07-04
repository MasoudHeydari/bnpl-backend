package checkoutsessionuc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type rateLimitKey struct {
	scope    model.OTPRateLimitScope
	scopeKey string
}

type fakeOTPRepo struct {
	mu         sync.Mutex
	byKey      map[string]model.OTPChallenge
	rateLimits map[rateLimitKey]struct {
		started time.Time
		count   int
	}
}

func newFakeOTPRepo() *fakeOTPRepo {
	return &fakeOTPRepo{
		byKey: make(map[string]model.OTPChallenge),
		rateLimits: make(map[rateLimitKey]struct {
			started time.Time
			count   int
		}),
	}
}

func challengeKey(purpose model.OTPPurpose, subjectKey string) string {
	return string(purpose) + "|" + subjectKey
}

func (f *fakeOTPRepo) Conn(repo.Conn) repo.OTPConnQueryer { return f }

func (f *fakeOTPRepo) Tx(repo.Tx) repo.OTPTxQueryer { return f }

func (f *fakeOTPRepo) GetByPurposeSubject(
	_ context.Context,
	purpose model.OTPPurpose,
	subjectKey string,
) (model.OTPChallenge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	row, ok := f.byKey[challengeKey(purpose, subjectKey)]
	if !ok {
		return model.OTPChallenge{}, cerr.ResourceNotFound(fmt.Errorf("otp challenge not found"))
	}

	return row, nil
}

func (f *fakeOTPRepo) EnsureAndLock(
	_ context.Context,
	purpose model.OTPPurpose,
	subjectKey string,
	now time.Time,
) (model.OTPChallenge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := challengeKey(purpose, subjectKey)
	row, ok := f.byKey[key]
	if !ok {
		row = model.OTPChallenge{
			ID:             model.OTPChallengeID(uuid.New()),
			Purpose:        purpose,
			SubjectKey:     subjectKey,
			ContextVersion: "",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		f.byKey[key] = row
	}

	return row, nil
}

func (f *fakeOTPRepo) Issue(
	_ context.Context,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
	verifier string,
	contextVersion string,
	expiresAt time.Time,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for key, row := range f.byKey {
		if row.ID != id {
			continue
		}

		if row.NextIssueAt != nil && updatedAt.Before(*row.NextIssueAt) {
			return model.OTPChallenge{}, cerr.ResourceNotFound(fmt.Errorf("issuance window active"))
		}

		issueCopy := issueID
		verifierCopy := verifier
		expiresCopy := expiresAt
		nextCopy := expiresAt
		row.IssueID = &issueCopy
		row.Verifier = &verifierCopy
		row.ContextVersion = contextVersion
		row.ExpiresAt = &expiresCopy
		row.NextIssueAt = &nextCopy
		row.NotificationAcceptedAt = nil
		row.VerifiedAt = nil
		row.NotificationRequestID = nil
		row.AttemptCount = 0
		row.SendCount++
		row.UpdatedAt = updatedAt
		f.byKey[key] = row

		return row, nil
	}

	return model.OTPChallenge{}, cerr.ResourceNotFound(fmt.Errorf("otp challenge not found"))
}

func (f *fakeOTPRepo) MarkNotificationAccepted(
	_ context.Context,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
	acceptedAt time.Time,
	requestID string,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for key, row := range f.byKey {
		if row.ID != id {
			continue
		}

		if row.IssueID == nil || *row.IssueID != issueID || row.Verifier == nil {
			return model.OTPChallenge{}, cerr.ResourceNotFound(fmt.Errorf("otp issue mismatch"))
		}

		acceptedCopy := acceptedAt
		requestCopy := requestID
		row.NotificationAcceptedAt = &acceptedCopy
		row.NotificationRequestID = &requestCopy
		row.UpdatedAt = updatedAt
		f.byKey[key] = row

		return row, nil
	}

	return model.OTPChallenge{}, cerr.ResourceNotFound(fmt.Errorf("otp challenge not found"))
}

func (f *fakeOTPRepo) ClearIssue(
	_ context.Context,
	id model.OTPChallengeID,
	issueID model.OTPIssueID,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for key, row := range f.byKey {
		if row.ID != id {
			continue
		}

		if row.IssueID == nil || *row.IssueID != issueID || row.VerifiedAt != nil {
			return model.OTPChallenge{}, cerr.ResourceNotFound(fmt.Errorf("otp issue mismatch"))
		}

		row.IssueID = nil
		row.Verifier = nil
		row.ExpiresAt = nil
		row.NextIssueAt = nil
		row.NotificationAcceptedAt = nil
		row.NotificationRequestID = nil
		row.AttemptCount = 0
		row.UpdatedAt = updatedAt
		f.byKey[key] = row

		return row, nil
	}

	return model.OTPChallenge{}, cerr.ResourceNotFound(fmt.Errorf("otp challenge not found"))
}

func (f *fakeOTPRepo) RecordFailure(
	_ context.Context,
	id model.OTPChallengeID,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for key, row := range f.byKey {
		if row.ID != id {
			continue
		}

		if row.Verifier == nil {
			return model.OTPChallenge{}, cerr.ResourceNotFound(fmt.Errorf("no verifier"))
		}

		row.AttemptCount++
		row.UpdatedAt = updatedAt
		f.byKey[key] = row

		return row, nil
	}

	return model.OTPChallenge{}, cerr.ResourceNotFound(fmt.Errorf("otp challenge not found"))
}

func (f *fakeOTPRepo) MarkVerified(
	_ context.Context,
	id model.OTPChallengeID,
	contextVersion string,
	verifiedAt time.Time,
	updatedAt time.Time,
) (model.OTPChallenge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for key, row := range f.byKey {
		if row.ID != id {
			continue
		}

		if row.Verifier == nil || row.ContextVersion != contextVersion {
			return model.OTPChallenge{}, cerr.ResourceNotFound(fmt.Errorf("otp verify mismatch"))
		}

		verifiedCopy := verifiedAt
		row.VerifiedAt = &verifiedCopy
		row.IssueID = nil
		row.Verifier = nil
		row.ExpiresAt = nil
		row.UpdatedAt = updatedAt
		f.byKey[key] = row

		return row, nil
	}

	return model.OTPChallenge{}, cerr.ResourceNotFound(fmt.Errorf("otp challenge not found"))
}

func (f *fakeOTPRepo) ConsumeRateLimit(
	_ context.Context,
	scope model.OTPRateLimitScope,
	scopeKey string,
	window time.Duration,
	maxSends int,
	now time.Time,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := rateLimitKey{scope: scope, scopeKey: scopeKey}
	existing, ok := f.rateLimits[key]
	if !ok || !existing.started.Add(window).After(now) {
		existing = struct {
			started time.Time
			count   int
		}{started: now, count: 0}
	}

	next := existing.count + 1
	if next > maxSends {
		return cerr.NewPublic(
			cerr.CodeTooManyRequests,
			"error.otp_rate_limited",
			fmt.Errorf("rate limited"),
			cerr.Details{},
		)
	}

	existing.count = next
	f.rateLimits[key] = existing

	return nil
}

// sequentialReader yields deterministic bytes so OTP generation and issue IDs
// are stable across a harness lifetime.
type sequentialReader struct {
	mu sync.Mutex
	n  uint64
}

func newSequentialReader() *sequentialReader {
	return &sequentialReader{}
}

func (r *sequentialReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range p {
		r.n++
		p[i] = byte(r.n)
	}

	return len(p), nil
}

