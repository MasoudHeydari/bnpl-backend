package financialcorepaymentresultuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

type checkoutActivationAuditFacts struct {
	InboxID        model.InboxID
	ActivateOutput contractuc.ActivateOutput
	Attempt        model.PaymentAttempt
	AppliedAt      time.Time
}

func buildCheckoutActivationAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	input ProcessInput,
	facts checkoutActivationAuditFacts,
	now time.Time,
) (audit.Event, error) {
	const op = "financialcorepaymentresultuc.buildCheckoutActivationAuditEvent"

	contract := facts.ActivateOutput.Contract
	first := facts.ActivateOutput.FirstInstallment
	remaining := facts.ActivateOutput.RemainingInstallments

	metadata := basePaymentResultAuditMetadata(input, facts.InboxID, ProcessResultPaymentApplied)
	reasonCodes := normalizedReasonCodes(auditReasonCheckoutFirstInstallmentPaymentSucceeded)
	putReasonCodes(metadata, reasonCodes)
	metadata["contract_id"] = contract.ID.String()
	metadata["decision_id"] = contract.DecisionID.String()
	metadata["customer_id"] = contract.CustomerID.String()
	metadata["merchant_profile_id"] = contract.MerchantProfileID.String()
	putOptionalUUIDPtr(metadata, "merchant_branch_profile_id", contract.MerchantBranchProfileID)
	metadata["first_installment_id"] = first.ID.String()
	metadata["first_installment_status"] = first.Status
	metadata["payment_attempt_id"] = facts.Attempt.ID.String()
	metadata["paid_charge_count"] = facts.ActivateOutput.PaidChargeCount
	metadata["applied_charge_count"] = facts.ActivateOutput.AppliedChargeCount
	metadata["added_active_exposure_amount_minor"] = facts.ActivateOutput.AddedActiveExposureAmountMinor
	metadata["remaining_principal_amount_minor"] = contract.RemainingPrincipalAmountMinor
	metadata["remaining_installment_count"] = len(remaining)
	metadata["applied_at"] = facts.AppliedAt.UTC()
	if contract.ActivatedAt != nil {
		metadata["contract_activated_at"] = contract.ActivatedAt.UTC()
	}

	remainingIDs := make([]string, 0, len(remaining))
	var scheduledCount, dueCount int
	for _, installment := range remaining {
		remainingIDs = append(remainingIDs, installment.ID.String())
		switch installment.Status {
		case model.ScheduledInstallmentStatus:
			scheduledCount++

		case model.DueInstallmentStatus:
			dueCount++
		}
	}

	metadata["remaining_installment_ids"] = remainingIDs
	metadata["scheduled_installment_count"] = scheduledCount
	metadata["due_installment_count"] = dueCount

	changes := []audit.FieldChange{
		{
			Field:  "contract.status",
			Before: model.PaymentPendingContractStatus,
			After:  model.ActiveContractStatus,
		},
		{
			Field:  "first_installment.status",
			Before: model.OfferedInstallmentStatus,
			After:  model.PaidInstallmentStatus,
		},
		{
			Field:  "remaining_installments.status",
			Before: model.OfferedInstallmentStatus,
			After:  "scheduled_or_due",
		},
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
			Action:     auditActionContractActivateFromCheckoutPayment,
			Resource:   audit.Resource{Type: contractResourceType, ID: contract.ID.String()},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: primaryReasonCode(reasonCodes),
			Changes:    changes,
			Metadata:   metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate checkout activation audit event: %w", op, err)
	}

	return event, nil
}

func (uc *UseCase) insertCheckoutActivationAudit(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	input ProcessInput,
	facts checkoutActivationAuditFacts,
	now time.Time,
) error {
	correlationID, traceID := resolveAuditEnvelopeIDs(ctx, input)
	event, err := buildCheckoutActivationAuditEvent(
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
