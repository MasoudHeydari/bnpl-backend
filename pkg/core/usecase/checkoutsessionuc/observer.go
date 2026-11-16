package checkoutsessionuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Observer records operational flow checkpoints for checkout session
// orchestration. Implementations must not modify domain state or return errors
// that affect business execution.
type Observer interface {
	SessionCreateRequested(ctx context.Context, externalMerchantID model.ExternalMerchantID)
	SessionInputRejected(ctx context.Context, reasonCode string)
	MerchantValidated(
		ctx context.Context,
		merchantProfileID model.MerchantProfileID,
		branchID model.MerchantBranchProfileID,
	)
	SessionCreated(ctx context.Context, sessionID model.CheckoutSessionID)
	SessionReplayed(ctx context.Context, sessionID model.CheckoutSessionID)
	SessionRetrieved(ctx context.Context, sessionID model.CheckoutSessionID)
	CustomerResolutionStarted(ctx context.Context, sessionID model.CheckoutSessionID)
	CustomerResolved(ctx context.Context, sessionID model.CheckoutSessionID, customerID model.CustomerID)
	CustomerResolutionFailed(ctx context.Context, sessionID model.CheckoutSessionID, reasonCode string)
	OfferPreparationStarted(ctx context.Context, sessionID model.CheckoutSessionID)
	OfferPrepared(ctx context.Context, sessionID model.CheckoutSessionID, decisionID model.DecisionID)
	OfferPreparationFailed(ctx context.Context, sessionID model.CheckoutSessionID, reasonCode string)
	SessionStarted(ctx context.Context, sessionID model.CheckoutSessionID)
	SessionStartReplayed(ctx context.Context, sessionID model.CheckoutSessionID)
	ConfirmRequested(ctx context.Context, sessionID model.CheckoutSessionID)
	ConfirmSagaStarted(ctx context.Context, sessionID model.CheckoutSessionID, sagaID string)
	ConfirmSagaStepStarted(ctx context.Context, sessionID model.CheckoutSessionID, step string)
	ConfirmSagaStepCompleted(ctx context.Context, sessionID model.CheckoutSessionID, step string)
	ConfirmSagaFailed(ctx context.Context, sessionID model.CheckoutSessionID, reasonCode string)
	ConfirmSagaCompensating(ctx context.Context, sessionID model.CheckoutSessionID, step string)
	SessionCanceled(ctx context.Context, sessionID model.CheckoutSessionID)
	SessionExpired(ctx context.Context, sessionID model.CheckoutSessionID)
	HostedHandoffPrepared(ctx context.Context, sessionID model.CheckoutSessionID)
	ExpirySweepCompleted(ctx context.Context, expiredCount int)
	ConfirmingRecoveryCompleted(ctx context.Context, recoveredCount int)
}
