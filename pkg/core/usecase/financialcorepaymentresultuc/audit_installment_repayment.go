package financialcorepaymentresultuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type installmentRepaymentAuditFacts struct {
	InboxID                     model.InboxID
	PreviousInstallmentStatus   model.InstallmentStatus
	CurrentInstallment          model.Installment
	PreviousPaymentAttemptState model.PaymentAttemptState
	CurrentPaymentAttempt       model.PaymentAttempt
	PreviousContractStatus      model.ContractStatus
	CurrentContractStatus       model.ContractStatus
	RemainingPrincipalMinor     uint64
	ContractCompleted           bool
	AppliedAt                   time.Time
	CustomerID                  model.CustomerID
	ContractID                  model.ContractID
}

func buildInstallmentRepaymentAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	input ProcessInput,
	facts installmentRepaymentAuditFacts,
	now time.Time,
) (audit.Event, error) {
	const op = "financialcorepaymentresultuc.buildInstallmentRepaymentAuditEvent"

	installment := facts.CurrentInstallment

	metadata := basePaymentResultAuditMetadata(input, facts.InboxID, ProcessResultPaymentApplied)
	reasonCodes := normalizedReasonCodes(auditReasonInstallmentRepaymentApplied)
	putReasonCodes(metadata, reasonCodes)
	metadata["installment_id"] = installment.ID.String()
	metadata["contract_id"] = facts.ContractID.String()
	metadata["customer_id"] = facts.CustomerID.String()
	metadata["payment_attempt_id"] = facts.CurrentPaymentAttempt.ID.String()
	metadata["installment_order"] = installment.InstallmentOrder
	metadata["previous_installment_status"] = facts.PreviousInstallmentStatus
	metadata["current_installment_status"] = installment.Status
	metadata["previous_payment_attempt_state"] = facts.PreviousPaymentAttemptState
	metadata["current_payment_attempt_state"] = facts.CurrentPaymentAttempt.State
	metadata["remaining_contract_principal_minor"] = facts.RemainingPrincipalMinor
	metadata["applied_at"] = facts.AppliedAt.UTC()
	if facts.ContractCompleted {
		metadata["contract_completed"] = true
	}

	changes := []audit.FieldChange{
		{
			Field:  "installment.status",
			Before: facts.PreviousInstallmentStatus,
			After:  installment.Status,
		},
		{
			Field:  "payment_attempt.state",
			Before: facts.PreviousPaymentAttemptState,
			After:  facts.CurrentPaymentAttempt.State,
		},
	}

	if facts.ContractCompleted {
		changes = append(changes, audit.FieldChange{
			Field:  "contract.status",
			Before: facts.PreviousContractStatus,
			After:  facts.CurrentContractStatus,
		})
	}

	causation := causationEventID(input)
	event := audit.NewEvent(
		eventID,
		correlationID,
		causation,
		traceID,
		now,
		audit.EventData{
			Actor:      auditActor(input.SourceService),
			Action:     auditActionInstallmentApplyRepayment,
			Resource:   audit.Resource{Type: installmentResourceType, ID: installment.ID.String()},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: primaryReasonCode(reasonCodes),
			Changes:    changes,
			Metadata:   metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate installment repayment audit event: %w", op, err)
	}

	return event, nil
}

func (uc *UseCase) insertInstallmentRepaymentAudit(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	input ProcessInput,
	facts installmentRepaymentAuditFacts,
	now time.Time,
) error {
	correlationID, traceID := resolveAuditEnvelopeIDs(ctx, input)
	event, err := buildInstallmentRepaymentAuditEvent(
		uc.newAuditEventID(),
		correlationID,
		traceID,
		input,
		facts,
		now,
	)
	if err != nil {
		return err
	}

	return uc.insertAuditOutbox(ctx, outboxQ, event, now)
}
