package observer

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/otp"
)

// OTP records OTP issuance and verification flow checkpoints.
type OTP struct {
	logger log.Logger
}

// NewOTP constructs an OTP observer.
func NewOTP(logger log.Logger) (*OTP, error) {
	const op = "observer.NewOTP"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &OTP{logger: logger}, nil
}

var _ otp.Observer = (*OTP)(nil)

// IssueRequested implements the corresponding Observer checkpoint.
func (o *OTP) IssueRequested(ctx context.Context, purpose model.OTPPurpose) {
	o.logger.WithContext(ctx).Info(
		event.OTPIssueRequested,
		field.Status(string(purpose)),
	)
}

// IssueRateLimited implements the corresponding Observer checkpoint.
func (o *OTP) IssueRateLimited(
	ctx context.Context,
	purpose model.OTPPurpose,
	challengeID model.OTPChallengeID,
) {
	fields := []log.Field{field.Status(string(purpose))}
	if !challengeID.IsZero() {
		fields = append(fields, field.OTPChallengeID(challengeID))
	}

	o.logger.WithContext(ctx).Warn(event.OTPIssueRateLimited, fields...)
}

// ChallengeReused implements the corresponding Observer checkpoint.
func (o *OTP) ChallengeReused(ctx context.Context, challengeID model.OTPChallengeID) {
	o.logger.WithContext(ctx).Info(event.OTPChallengeReused, field.OTPChallengeID(challengeID))
}

// ChallengeIssued implements the corresponding Observer checkpoint.
func (o *OTP) ChallengeIssued(ctx context.Context, challengeID model.OTPChallengeID) {
	o.logger.WithContext(ctx).Info(event.OTPChallengeIssued, field.OTPChallengeID(challengeID))
}

// NotificationSubmitStarted implements the corresponding Observer checkpoint.
func (o *OTP) NotificationSubmitStarted(
	ctx context.Context,
	challengeID model.OTPChallengeID,
	templateID string,
) {
	o.logger.WithContext(ctx).Info(
		event.OTPNotificationSubmitStarted,
		field.OTPChallengeID(challengeID),
		field.TemplateID(templateID),
	)
}

// ProviderAccepted implements the corresponding Observer checkpoint.
func (o *OTP) ProviderAccepted(
	ctx context.Context,
	challengeID model.OTPChallengeID,
	templateID string,
) {
	o.logger.WithContext(ctx).Info(
		event.OTPProviderAccepted,
		field.OTPChallengeID(challengeID),
		field.TemplateID(templateID),
	)
}

// ProviderRejected implements the corresponding Observer checkpoint.
func (o *OTP) ProviderRejected(
	ctx context.Context,
	challengeID model.OTPChallengeID,
	reasonCode string,
) {
	o.logger.WithContext(ctx).Warn(
		event.OTPProviderRejected,
		field.OTPChallengeID(challengeID),
		field.ReasonCode(reasonCode),
	)
}

// ProviderTimedOut implements the corresponding Observer checkpoint.
func (o *OTP) ProviderTimedOut(ctx context.Context, challengeID model.OTPChallengeID) {
	o.logger.WithContext(ctx).Warn(
		event.OTPProviderTimedOut,
		field.OTPChallengeID(challengeID),
	)
}

// VerificationAttempted implements the corresponding Observer checkpoint.
func (o *OTP) VerificationAttempted(ctx context.Context, challengeID model.OTPChallengeID) {
	o.logger.WithContext(ctx).Info(
		event.OTPVerificationAttempted,
		field.OTPChallengeID(challengeID),
	)
}

// VerificationSucceeded implements the corresponding Observer checkpoint.
func (o *OTP) VerificationSucceeded(ctx context.Context, challengeID model.OTPChallengeID) {
	o.logger.WithContext(ctx).Info(
		event.OTPVerificationSucceeded,
		field.OTPChallengeID(challengeID),
	)
}

// VerificationFailed implements the corresponding Observer checkpoint.
func (o *OTP) VerificationFailed(
	ctx context.Context,
	challengeID model.OTPChallengeID,
	reasonCode string,
) {
	o.logger.WithContext(ctx).Info(
		event.OTPVerificationFailed,
		field.OTPChallengeID(challengeID),
		field.ReasonCode(reasonCode),
	)
}

// ChallengeExpired implements the corresponding Observer checkpoint.
func (o *OTP) ChallengeExpired(ctx context.Context, challengeID model.OTPChallengeID) {
	o.logger.WithContext(ctx).Info(
		event.OTPChallengeExpired,
		field.OTPChallengeID(challengeID),
	)
}

// ChallengeLocked implements the corresponding Observer checkpoint.
func (o *OTP) ChallengeLocked(ctx context.Context, challengeID model.OTPChallengeID) {
	o.logger.WithContext(ctx).Warn(
		event.OTPChallengeLocked,
		field.OTPChallengeID(challengeID),
	)
}
