package otp

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/external/notification"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	otppolicy "github.com/parstis/bnpl-backend/pkg/core/policy/otp"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	// defaultNotificationDeliveryTimeout bounds post-commit Notification Submit
	// and MarkAccepted after Issue commits. Mirrors sagaexecutoruc persistence
	// detachment so an HTTP cancel cannot strand delivery indefinitely.
	defaultNotificationDeliveryTimeout = 15 * time.Second
)

// UseCase issues, verifies, and projects reusable OTP challenges.
type UseCase struct {
	pool                repo.Pool
	challenges          repo.OTP
	hasher              fingerprint.Hasher
	notification        notification.Submitter
	policy              otppolicy.Policy
	clock               clock.Clock
	random              io.Reader
	notificationTimeout time.Duration

	// beforeIssueCommit is an optional test barrier invoked inside the Issue
	// transaction after eligibility and rate-limit consumption and before the
	// durable Issue write. Production leaves this nil.
	beforeIssueCommit func(context.Context) error

	// afterNotifyProject is an optional test hook invoked after Notification
	// acceptance when projecting the final Request output. When it returns an
	// error, Request still returns OTPSent with timers (notify already happened).
	afterNotifyProject func(context.Context) error
}

// New constructs an OTP use case.
//
// After Issue commits, Notification delivery uses a detached context with
// notificationTimeout (default 15s). Plaintext codes are never stored: if the
// process crashes between Issue and MarkAccepted, the unused challenge occupies
// the issuance window until expiry; a later Request replays it without a second
// SMS while the challenge remains usable. ClearIssue runs only on an explicit
// Notification Rejected outcome — ambiguous timeout/cancel failures leave the
// challenge intact for retry.
func New(
	pool repo.Pool,
	challenges repo.OTP,
	hasher fingerprint.Hasher,
	notificationSubmitter notification.Submitter,
	policy otppolicy.Policy,
	clk clock.Clock,
	random io.Reader,
) (*UseCase, error) {
	return NewWithNotificationTimeout(
		pool,
		challenges,
		hasher,
		notificationSubmitter,
		policy,
		clk,
		random,
		defaultNotificationDeliveryTimeout,
	)
}

// NewWithNotificationTimeout constructs an OTP use case with an explicit
// post-commit notification delivery timeout. Non-positive timeout falls back to
// the default.
func NewWithNotificationTimeout(
	pool repo.Pool,
	challenges repo.OTP,
	hasher fingerprint.Hasher,
	notificationSubmitter notification.Submitter,
	policy otppolicy.Policy,
	clk clock.Clock,
	random io.Reader,
	notificationTimeout time.Duration,
) (*UseCase, error) {
	const op = "otp.New"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if challenges == nil {
		return nil, fmt.Errorf("%s: otp challenge repository is required", op)
	}

	if hasher == nil {
		return nil, fmt.Errorf("%s: otp hasher is required", op)
	}

	if notificationSubmitter == nil {
		return nil, fmt.Errorf("%s: notification submitter is required", op)
	}

	if policy == nil {
		return nil, fmt.Errorf("%s: otp policy is required", op)
	}

	if err := policy.Terms().Validate(); err != nil {
		return nil, fmt.Errorf("%s: otp policy terms: %w", op, err)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if random == nil {
		random = rand.Reader
	}

	if notificationTimeout <= 0 {
		notificationTimeout = defaultNotificationDeliveryTimeout
	}

	return &UseCase{
		pool:                pool,
		challenges:          challenges,
		hasher:              hasher,
		notification:        notificationSubmitter,
		policy:              policy,
		clock:               clk,
		random:              random,
		notificationTimeout: notificationTimeout,
	}, nil
}

// Terms returns the immutable OTP security terms enforced by this use case.
func (uc *UseCase) Terms() otppolicy.Terms {
	return uc.policy.Terms()
}

// SetBeforeIssueCommit installs a test barrier inside the Issue transaction.
// Production callers must leave this unset.
func (uc *UseCase) SetBeforeIssueCommit(fn func(context.Context) error) {
	uc.beforeIssueCommit = fn
}

// SetAfterNotifyProjectForTest installs a post-notify projection hook used by
// unit tests to assert that Notification still runs when projection fails.
func (uc *UseCase) SetAfterNotifyProjectForTest(fn func(context.Context) error) {
	uc.afterNotifyProject = fn
}

// notificationDeliveryContext detaches cancellation from the caller so
// post-commit Notification Submit / MarkAccepted / ClearIssue-on-reject can
// finish after an HTTP cancel, and bounds that work with timeout.
func notificationDeliveryContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := ctx
	if base == nil {
		base = context.Background()
	} else {
		base = context.WithoutCancel(base)
	}

	if timeout <= 0 {
		return base, func() {}
	}

	return context.WithTimeout(base, timeout)
}
