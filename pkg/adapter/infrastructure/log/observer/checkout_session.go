package observer

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// CheckoutSession records checkout-session flow checkpoints.
type CheckoutSession struct {
	logger log.Logger
}

// NewCheckoutSession constructs a checkout-session observer.
func NewCheckoutSession(logger log.Logger) (*CheckoutSession, error) {
	const op = "observer.NewCheckoutSession"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &CheckoutSession{logger: logger}, nil
}

var _ checkoutsessionuc.Observer = (*CheckoutSession)(nil)

// SessionCreateRequested implements the corresponding Observer checkpoint.
func (o *CheckoutSession) SessionCreateRequested(
	ctx context.Context,
	externalMerchantID model.ExternalMerchantID,
) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutSessionCreateRequested,
		field.ExternalMerchantID(externalMerchantID),
	)
}

// SessionInputRejected implements the corresponding Observer checkpoint.
func (o *CheckoutSession) SessionInputRejected(ctx context.Context, reasonCode string) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutSessionInputRejected,
		field.ReasonCode(reasonCode),
	)
}

// MerchantValidated implements the corresponding Observer checkpoint.
func (o *CheckoutSession) MerchantValidated(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	branchID model.MerchantBranchProfileID,
) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutMerchantValidated,
		field.MerchantProfileID(merchantProfileID),
		field.MerchantBranchID(branchID),
	)
}

// SessionCreated implements the corresponding Observer checkpoint.
func (o *CheckoutSession) SessionCreated(ctx context.Context, sessionID model.CheckoutSessionID) {
	o.logger.WithContext(ctx).Info(event.CheckoutSessionCreated, field.SessionID(sessionID))
}

// SessionReplayed implements the corresponding Observer checkpoint.
func (o *CheckoutSession) SessionReplayed(ctx context.Context, sessionID model.CheckoutSessionID) {
	o.logger.WithContext(ctx).Info(event.CheckoutSessionReplayed, field.SessionID(sessionID))
}

// SessionRetrieved implements the corresponding Observer checkpoint.
func (o *CheckoutSession) SessionRetrieved(ctx context.Context, sessionID model.CheckoutSessionID) {
	o.logger.WithContext(ctx).Debug(event.CheckoutSessionRetrieved, field.SessionID(sessionID))
}

// CustomerResolutionStarted implements the corresponding Observer checkpoint.
func (o *CheckoutSession) CustomerResolutionStarted(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutCustomerResolutionStarted,
		field.SessionID(sessionID),
	)
}

// CustomerResolved implements the corresponding Observer checkpoint.
func (o *CheckoutSession) CustomerResolved(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	customerID model.CustomerID,
) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutCustomerResolved,
		field.SessionID(sessionID),
		field.CustomerID(customerID),
	)
}

// CustomerResolutionFailed implements the corresponding Observer checkpoint.
func (o *CheckoutSession) CustomerResolutionFailed(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	reasonCode string,
) {
	o.logger.WithContext(ctx).Warn(
		event.CheckoutCustomerResolutionFailed,
		field.SessionID(sessionID),
		field.ReasonCode(reasonCode),
	)
}

// OfferPreparationStarted implements the corresponding Observer checkpoint.
func (o *CheckoutSession) OfferPreparationStarted(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutOfferPreparationStarted,
		field.SessionID(sessionID),
	)
}

// OfferPrepared implements the corresponding Observer checkpoint.
func (o *CheckoutSession) OfferPrepared(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	decisionID model.DecisionID,
) {
	fields := []log.Field{field.SessionID(sessionID)}
	if !decisionID.IsZero() {
		fields = append(fields, field.DecisionID(decisionID))
	}

	o.logger.WithContext(ctx).Info(event.CheckoutOfferPrepared, fields...)
}

// OfferPreparationFailed implements the corresponding Observer checkpoint.
func (o *CheckoutSession) OfferPreparationFailed(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	reasonCode string,
) {
	o.logger.WithContext(ctx).Warn(
		event.CheckoutOfferPreparationFailed,
		field.SessionID(sessionID),
		field.ReasonCode(reasonCode),
	)
}

// SessionStarted implements the corresponding Observer checkpoint.
func (o *CheckoutSession) SessionStarted(ctx context.Context, sessionID model.CheckoutSessionID) {
	o.logger.WithContext(ctx).Info(event.CheckoutSessionStarted, field.SessionID(sessionID))
}

// SessionStartReplayed implements the corresponding Observer checkpoint.
func (o *CheckoutSession) SessionStartReplayed(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutSessionStartReplayed,
		field.SessionID(sessionID),
	)
}

// ConfirmRequested implements the corresponding Observer checkpoint.
func (o *CheckoutSession) ConfirmRequested(ctx context.Context, sessionID model.CheckoutSessionID) {
	o.logger.WithContext(ctx).Info(event.CheckoutConfirmRequested, field.SessionID(sessionID))
}

// ConfirmSagaStarted implements the corresponding Observer checkpoint.
func (o *CheckoutSession) ConfirmSagaStarted(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	sagaID string,
) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutConfirmSagaStarted,
		field.SessionID(sessionID),
		field.SagaID(sagaID),
	)
}

// ConfirmSagaStepStarted implements the corresponding Observer checkpoint.
func (o *CheckoutSession) ConfirmSagaStepStarted(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	step string,
) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutConfirmSagaStepStarted,
		field.SessionID(sessionID),
		field.SagaStep(step),
	)
}

// ConfirmSagaStepCompleted implements the corresponding Observer checkpoint.
func (o *CheckoutSession) ConfirmSagaStepCompleted(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	step string,
) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutConfirmSagaStepCompleted,
		field.SessionID(sessionID),
		field.SagaStep(step),
	)
}

// ConfirmSagaFailed implements the corresponding Observer checkpoint.
func (o *CheckoutSession) ConfirmSagaFailed(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	reasonCode string,
) {
	o.logger.WithContext(ctx).Warn(
		event.CheckoutConfirmSagaFailed,
		field.SessionID(sessionID),
		field.ReasonCode(reasonCode),
	)
}

// ConfirmSagaCompensating implements the corresponding Observer checkpoint.
func (o *CheckoutSession) ConfirmSagaCompensating(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	step string,
) {
	o.logger.WithContext(ctx).Warn(
		event.CheckoutConfirmSagaCompensating,
		field.SessionID(sessionID),
		field.SagaStep(step),
	)
}

// SessionCanceled implements the corresponding Observer checkpoint.
func (o *CheckoutSession) SessionCanceled(ctx context.Context, sessionID model.CheckoutSessionID) {
	o.logger.WithContext(ctx).Info(event.CheckoutSessionCanceled, field.SessionID(sessionID))
}

// SessionExpired implements the corresponding Observer checkpoint.
func (o *CheckoutSession) SessionExpired(ctx context.Context, sessionID model.CheckoutSessionID) {
	o.logger.WithContext(ctx).Info(event.CheckoutSessionExpired, field.SessionID(sessionID))
}

// CheckoutTerminatedSafely implements the corresponding Observer checkpoint.
func (o *CheckoutSession) CheckoutTerminatedSafely(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	terminalStatus string,
	reason string,
) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutTerminatedSafely,
		field.SessionID(sessionID),
		field.Status(terminalStatus),
		field.ReasonCode(reason),
	)
}

// CheckoutSafeTerminationRejected implements the corresponding Observer checkpoint.
func (o *CheckoutSession) CheckoutSafeTerminationRejected(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
	blockReason string,
) {
	o.logger.WithContext(ctx).Warn(
		event.CheckoutSafeTerminationRejected,
		field.SessionID(sessionID),
		field.ReasonCode(blockReason),
	)
}

// HostedHandoffPrepared implements the corresponding Observer checkpoint.
func (o *CheckoutSession) HostedHandoffPrepared(
	ctx context.Context,
	sessionID model.CheckoutSessionID,
) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutHostedHandoffPrepared,
		field.SessionID(sessionID),
	)
}

// ExpirySweepCompleted implements the corresponding Observer checkpoint.
func (o *CheckoutSession) ExpirySweepCompleted(ctx context.Context, expiredCount int) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutSessionExpirySweepCompleted,
		field.ProcessedCount(expiredCount),
	)
}

// ConfirmingRecoveryCompleted implements the corresponding Observer checkpoint.
func (o *CheckoutSession) ConfirmingRecoveryCompleted(ctx context.Context, recoveredCount int) {
	o.logger.WithContext(ctx).Info(
		event.CheckoutConfirmingRecoveryCompleted,
		field.ProcessedCount(recoveredCount),
	)
}
