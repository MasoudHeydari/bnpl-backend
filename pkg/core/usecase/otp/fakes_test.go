package otp_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	fingerprinthmac "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/hmac"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/otpcode"
	otppolicy "github.com/parstis/bnpl-backend/pkg/core/policy/otp"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
)

const (
	testPurpose        = model.OTPPurposeHostedCheckoutConfirm
	testSubjectKey     = "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testContextVersion = "1"
	testBinding        = testSubjectKey
	testRecipient      = "+989121234567"
	testTemplateID     = "bnpl.test_otp.v1"
	testIdempotency    = "otp-test:"
)

type harness struct {
	t            *testing.T
	ctx          context.Context
	now          time.Time
	uc           *otp.UseCase
	pool         *mock.Pool
	conn         *mock.Conn
	tx           *mock.Tx
	challenges   *fakeOTPRepo
	hasher       fingerprint.Hasher
	notification *fakeNotification
	authorizer   *fakeAuthorizer
	random       *sequentialReader
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	hasher, err := fingerprinthmac.New([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)

	now := time.Date(
		2026,
		time.March,
		20,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	h := &harness{
		t:            t,
		ctx:          context.Background(),
		now:          now,
		pool:         mock.NewPool(t),
		conn:         mock.NewConn(t),
		tx:           mock.NewTx(t),
		challenges:   newFakeOTPRepo(),
		hasher:       hasher,
		notification: &fakeNotification{},
		authorizer:   newFakeAuthorizer(),
		random:       newSequentialReader(),
	}

	h.pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, h.conn)
		}).Maybe()
	h.conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, h.tx)
		}).Maybe()

	h.uc, err = otp.New(
		h.pool,
		h.challenges,
		h.hasher,
		h.notification,
		otppolicy.V1(),
		adapterclock.NewFixed(now, time.UTC),
		h.random,
	)
	require.NoError(t, err)

	return h
}

func (h *harness) advance(d time.Duration) {
	h.t.Helper()

	h.now = h.now.Add(d)
	h.uc, _ = otp.New(
		h.pool,
		h.challenges,
		h.hasher,
		h.notification,
		otppolicy.V1(),
		adapterclock.NewFixed(h.now, time.UTC),
		h.random,
	)
}

func (h *harness) request() (otp.RequestOutput, error) {
	h.t.Helper()

	return h.uc.Request(h.ctx, otp.RequestInput{
		Purpose:        testPurpose,
		SubjectKey:     testSubjectKey,
		ContextVersion: testContextVersion,
		Authorizer:     h.authorizer,
	})
}

func (h *harness) verify(code string) (otp.VerifyOutput, error) {
	h.t.Helper()

	return h.uc.VerifyAndConsume(h.ctx, otp.VerifyInput{
		Purpose:        testPurpose,
		SubjectKey:     testSubjectKey,
		ContextVersion: testContextVersion,
		Binding:        testBinding,
		Code:           code,
	})
}

type fakeAuthorizer struct {
	terminal        bool
	businessBlocked bool
	rateLimits      []otp.RateLimitBudget
	err             error
}

func newFakeAuthorizer() *fakeAuthorizer {
	return &fakeAuthorizer{
		rateLimits: []otp.RateLimitBudget{
			{
				Scope:    model.OTPRateLimitScopePhone,
				ScopeKey: "phone-fp",
				Window:   10 * time.Minute,
				MaxSends: 5,
			},
		},
	}
}

func (f *fakeAuthorizer) AuthorizeIssuance(context.Context, repo.Tx) (otp.AuthorizedIssuance, error) {
	if f.err != nil {
		return otp.AuthorizedIssuance{}, f.err
	}

	return otp.AuthorizedIssuance{
		Purpose:              testPurpose,
		SubjectKey:           testSubjectKey,
		ContextVersion:       testContextVersion,
		Binding:              testBinding,
		Recipient:            testRecipient,
		TemplateID:           testTemplateID,
		Variables:            map[string]string{"merchant_name": "TestMerchant"},
		IdempotencyKeyPrefix: testIdempotency,
		CorrelationID:        "corr-1",
		RateLimits:           f.rateLimits,
		Terminal:             f.terminal,
		BusinessBlocked:      f.businessBlocked,
	}, nil
}

type fakeNotification struct {
	mu       sync.Mutex
	submits  []notification.SubmitInput
	err      error
	requestN int

	// blockUntil, when non-nil, is closed by the test to unblock Submit. Submit
	// waits on it while observing ctx so detached-delivery tests can prove the
	// parent cancel does not abort delivery.
	blockUntil <-chan struct{}
	sawCtx     context.Context
}

func (f *fakeNotification) Submit(ctx context.Context, input notification.SubmitInput) (notification.SubmitOutput, error) {
	f.mu.Lock()
	f.sawCtx = ctx
	blockUntil := f.blockUntil
	f.mu.Unlock()

	if blockUntil != nil {
		select {
		case <-blockUntil:
		case <-ctx.Done():
			return notification.SubmitOutput{}, ctx.Err()
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.submits = append(f.submits, input)
	if f.err != nil {
		return notification.SubmitOutput{}, f.err
	}

	f.requestN++

	return notification.SubmitOutput{RequestID: fmt.Sprintf("req-%d", f.requestN)}, nil
}

func (f *fakeNotification) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.submits)
}

func (f *fakeNotification) last() notification.SubmitInput {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.submits[len(f.submits)-1]
}

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
	rateLimitOrder []rateLimitKey
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
	f.rateLimitOrder = append(f.rateLimitOrder, key)
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

func mustCodeFromSubmit(t *testing.T, n *fakeNotification) string {
	t.Helper()

	require.Greater(t, n.count(), 0)
	code := n.last().Variables["code"]
	require.True(t, otpcode.IsWellFormed(code))

	return code
}
