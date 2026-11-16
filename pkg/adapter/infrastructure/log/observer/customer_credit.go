package observer

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

// CustomerCredit records customer credit assessment and limit flow checkpoints.
type CustomerCredit struct {
	logger log.Logger
}

// NewCustomerCredit constructs a customer credit observer.
func NewCustomerCredit(logger log.Logger) (*CustomerCredit, error) {
	const op = "observer.NewCustomerCredit"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &CustomerCredit{logger: logger}, nil
}

var _ customercredituc.Observer = (*CustomerCredit)(nil)

// CustomerResolved implements the corresponding Observer checkpoint.
func (o *CustomerCredit) CustomerResolved(ctx context.Context, externalCustomerID model.ExternalCustomerID) {
	o.logger.WithContext(ctx).Info(
		event.CreditCustomerResolved,
		field.ExternalCustomerID(externalCustomerID),
	)
}

// AssessmentStarted implements the corresponding Observer checkpoint.
func (o *CustomerCredit) AssessmentStarted(ctx context.Context, customerID model.CustomerID) {
	o.logger.WithContext(ctx).Info(
		event.CreditAssessmentStarted,
		field.CustomerID(customerID),
	)
}

// AssessmentCompleted implements the corresponding Observer checkpoint.
func (o *CustomerCredit) AssessmentCompleted(
	ctx context.Context,
	customerID model.CustomerID,
	outcome model.DecisionOutcome,
) {
	o.logger.WithContext(ctx).Info(
		event.CreditAssessmentCompleted,
		field.CustomerID(customerID),
		field.Outcome(string(outcome)),
	)
}

// LimitAssignmentAttempted implements the corresponding Observer checkpoint.
func (o *CustomerCredit) LimitAssignmentAttempted(
	ctx context.Context,
	externalSubjectID model.ExternalSubjectID,
) {
	o.logger.WithContext(ctx).Info(
		event.CreditLimitAssignmentAttempted,
		field.ExternalSubjectID(externalSubjectID),
	)
}

// LimitAssigned implements the corresponding Observer checkpoint.
func (o *CustomerCredit) LimitAssigned(
	ctx context.Context,
	customerID model.CustomerID,
	decisionID model.DecisionID,
) {
	o.logger.WithContext(ctx).Info(
		event.CreditLimitAssigned,
		field.CustomerID(customerID),
		field.DecisionID(decisionID),
	)
}

// LimitAlreadyAssigned implements the corresponding Observer checkpoint.
func (o *CustomerCredit) LimitAlreadyAssigned(
	ctx context.Context,
	customerID model.CustomerID,
	decisionID model.DecisionID,
) {
	o.logger.WithContext(ctx).Info(
		event.CreditLimitAlreadyAssigned,
		field.CustomerID(customerID),
		field.DecisionID(decisionID),
	)
}

// LimitAssignmentDenied implements the corresponding Observer checkpoint.
func (o *CustomerCredit) LimitAssignmentDenied(
	ctx context.Context,
	customerID model.CustomerID,
	decisionID model.DecisionID,
	reasonCode string,
) {
	fields := []log.Field{
		field.CustomerID(customerID),
		field.DecisionID(decisionID),
	}
	if reasonCode != "" {
		fields = append(fields, field.ReasonCode(reasonCode))
	}

	o.logger.WithContext(ctx).Info(event.CreditLimitAssignmentDenied, fields...)
}

// AvailabilityRetrieved implements the corresponding Observer checkpoint.
func (o *CustomerCredit) AvailabilityRetrieved(
	ctx context.Context,
	customerID model.CustomerID,
	status string,
) {
	o.logger.WithContext(ctx).Info(
		event.CreditAvailabilityRetrieved,
		field.CustomerID(customerID),
		field.Status(status),
	)
}

// UsageRestricted implements the corresponding Observer checkpoint.
func (o *CustomerCredit) UsageRestricted(
	ctx context.Context,
	customerID model.CustomerID,
	reasonCode string,
) {
	fields := []log.Field{field.CustomerID(customerID)}
	if reasonCode != "" {
		fields = append(fields, field.ReasonCode(reasonCode))
	}

	o.logger.WithContext(ctx).Info(event.CreditUsageRestricted, fields...)
}
