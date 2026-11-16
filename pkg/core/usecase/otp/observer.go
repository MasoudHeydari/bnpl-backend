package otp

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Observer receives privacy-safe OTP flow checkpoints for operational logging.
//
// Implementations must never record plaintext codes, verifiers, or phone
// numbers.
type Observer interface {
	IssueRequested(ctx context.Context, purpose model.OTPPurpose)
	IssueRateLimited(ctx context.Context, purpose model.OTPPurpose, challengeID model.OTPChallengeID)
	ChallengeReused(ctx context.Context, challengeID model.OTPChallengeID)
	ChallengeIssued(ctx context.Context, challengeID model.OTPChallengeID)
	NotificationSubmitStarted(ctx context.Context, challengeID model.OTPChallengeID, templateID string)
	ProviderAccepted(ctx context.Context, challengeID model.OTPChallengeID, templateID string)
	ProviderRejected(ctx context.Context, challengeID model.OTPChallengeID, reasonCode string)
	ProviderTimedOut(ctx context.Context, challengeID model.OTPChallengeID)
	VerificationAttempted(ctx context.Context, challengeID model.OTPChallengeID)
	VerificationSucceeded(ctx context.Context, challengeID model.OTPChallengeID)
	VerificationFailed(ctx context.Context, challengeID model.OTPChallengeID, reasonCode string)
	ChallengeExpired(ctx context.Context, challengeID model.OTPChallengeID)
	ChallengeLocked(ctx context.Context, challengeID model.OTPChallengeID)
}
