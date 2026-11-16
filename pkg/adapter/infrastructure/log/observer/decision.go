package observer

import (
	"context"
	"fmt"
	"math"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

// Decision records checkout decision evaluation flow checkpoints.
type Decision struct {
	logger log.Logger
}

// NewDecision constructs a decision observer.
func NewDecision(logger log.Logger) (*Decision, error) {
	const op = "observer.NewDecision"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &Decision{logger: logger}, nil
}

var _ decisionuc.Observer = (*Decision)(nil)

// EvaluationStarted implements the corresponding Observer checkpoint.
func (o *Decision) EvaluationStarted(ctx context.Context, externalSubjectID model.ExternalSubjectID) {
	o.logger.WithContext(ctx).Info(
		event.DecisionEvaluationStarted,
		field.ExternalSubjectID(externalSubjectID),
	)
}

// AvailableCreditChecked implements the corresponding Observer checkpoint.
func (o *Decision) AvailableCreditChecked(
	ctx context.Context,
	decisionID model.DecisionID,
	availableAmountMinor uint64,
	currency model.Currency,
) {
	o.logger.WithContext(ctx).Info(
		event.DecisionAvailableCreditChecked,
		field.DecisionID(decisionID),
		field.AmountMinor(decisionAmountMinor(availableAmountMinor)),
		field.Currency(string(currency)),
	)
}

// DecisionApproved implements the corresponding Observer checkpoint.
func (o *Decision) DecisionApproved(ctx context.Context, decisionID model.DecisionID) {
	o.logger.WithContext(ctx).Info(event.DecisionApproved, field.DecisionID(decisionID))
}

// DecisionPartiallyApproved implements the corresponding Observer checkpoint.
func (o *Decision) DecisionPartiallyApproved(ctx context.Context, decisionID model.DecisionID) {
	o.logger.WithContext(ctx).Info(event.DecisionPartiallyApproved, field.DecisionID(decisionID))
}

// DecisionDenied implements the corresponding Observer checkpoint.
func (o *Decision) DecisionDenied(ctx context.Context, decisionID model.DecisionID, reasonCode string) {
	fields := []log.Field{field.DecisionID(decisionID)}
	if reasonCode != "" {
		fields = append(fields, field.ReasonCode(reasonCode))
	}

	o.logger.WithContext(ctx).Info(event.DecisionDenied, fields...)
}

// DecisionPersisted implements the corresponding Observer checkpoint.
func (o *Decision) DecisionPersisted(
	ctx context.Context,
	decisionID model.DecisionID,
	outcome model.DecisionOutcome,
) {
	o.logger.WithContext(ctx).Info(
		event.DecisionPersisted,
		field.DecisionID(decisionID),
		field.Outcome(string(outcome)),
	)
}

// DecisionReplayed implements the corresponding Observer checkpoint.
func (o *Decision) DecisionReplayed(
	ctx context.Context,
	decisionID model.DecisionID,
	outcome model.DecisionOutcome,
) {
	o.logger.WithContext(ctx).Info(
		event.DecisionReplayed,
		field.DecisionID(decisionID),
		field.Outcome(string(outcome)),
	)
}

// OfferExpired implements the corresponding Observer checkpoint.
func (o *Decision) OfferExpired(ctx context.Context, decisionID model.DecisionID) {
	o.logger.WithContext(ctx).Info(event.DecisionOfferExpired, field.DecisionID(decisionID))
}

// OfferInvalidated implements the corresponding Observer checkpoint.
func (o *Decision) OfferInvalidated(ctx context.Context, decisionID model.DecisionID, reasonCode string) {
	o.logger.WithContext(ctx).Info(
		event.DecisionOfferInvalidated,
		field.DecisionID(decisionID),
		field.ReasonCode(reasonCode),
	)
}

// decisionAmountMinor converts a uint64 minor-unit amount to the int64 wire
// attribute, saturating at math.MaxInt64 (checkout amounts are already bounded
// by that limit at input validation).
func decisionAmountMinor(amount uint64) int64 {
	if amount > math.MaxInt64 {
		return math.MaxInt64
	}

	return int64(amount)
}
