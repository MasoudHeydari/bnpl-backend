package decisionuc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	checkoutDecisionAuditAction  = "checkout_decision.create"
	checkoutDecisionResourceType = "checkout_decision"
)

// checkoutDecisionAuditFacts carries in-transaction identifiers already loaded
// for the audited Decision. Do not query repositories to populate this struct.
type checkoutDecisionAuditFacts struct {
	ExternalCustomerID       model.ExternalCustomerID
	CustomerID              model.CustomerID
	MerchantProfileID       model.MerchantProfileID
	MerchantID              model.MerchantID
	MerchantBranchProfileID model.MerchantBranchProfileID
	BranchID                model.BranchID
	PurchaseChannel         model.PurchaseChannel
	PurchaseSourceID        uuid.UUID
	MerchantOrderID         *string
}

// insertCheckoutDecisionAuditOutbox records the concluded checkout decision in
// the transactional outbox. Call only for newly created decisions, not replay.
//
// Actor is the authenticated customer (Identity subject / customer_profiles.external_customer_id).
// Decision.ActorType remains system and is recorded in Metadata as decision_actor_type.
func (uc *UseCase) insertCheckoutDecisionAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	decision model.Decision,
	output CreateOutput,
	facts checkoutDecisionAuditFacts,
	now time.Time,
) error {
	const op = "decisionuc.UseCase.insertCheckoutDecisionAuditOutbox"

	correlationID, traceID, err := audit.EnvelopeIDsFromContext(ctx)
	if err != nil {
		return fmt.Errorf("%s: checkout decision audit envelope ids: %w", op, err)
	}

	event, err := buildCheckoutDecisionAuditEvent(
		uc.idGenerator.Generate(),
		correlationID,
		traceID,
		decision,
		output,
		facts,
		now,
	)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal checkout decision audit event: %w", op, err)
	}

	aggregateType := checkoutDecisionResourceType
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    decision.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert checkout decision audit outbox message: %w", op, err)
	}

	return nil
}

func buildCheckoutDecisionAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	decision model.Decision,
	output CreateOutput,
	facts checkoutDecisionAuditFacts,
	now time.Time,
) (audit.Event, error) {
	const op = "decisionuc.buildCheckoutDecisionAuditEvent"

	actorID := facts.ExternalCustomerID.String()
	reasonCodes := decision.ReasonCodes

	metadata := map[string]any{
		"decision_id":                decision.ID.String(),
		"checkout_key":               decision.IdempotencyKey,
		"customer_id":                facts.CustomerID.String(),
		"identity_subject_id":        facts.ExternalCustomerID.String(),
		"merchant_profile_id":        facts.MerchantProfileID.String(),
		"merchant_id":                facts.MerchantID.String(),
		"merchant_branch_profile_id": facts.MerchantBranchProfileID.String(),
		"branch_id":                  facts.BranchID.String(),
		"purchase_channel":           facts.PurchaseChannel,
		"purchase_source_id":         facts.PurchaseSourceID.String(),
		"decision_type":              decision.Type,
		"decision_outcome":           decision.Outcome,
		"decision_actor_type":        decision.ActorType,
		"reason_codes":               append([]string(nil), reasonCodes...),
		"policy_id":                  decision.PolicyID,
		"policy_version":             decision.PolicyVersion,
		"requested_amount_minor":     output.RequestedAmountMinor,
		"currency":                   output.Currency,
	}

	if facts.MerchantOrderID != nil {
		metadata["merchant_order_id"] = *facts.MerchantOrderID
	}

	if output.CheckoutAction != nil {
		metadata["checkout_action"] = output.CheckoutAction.Type
		metadata["initial_payment_required"] = output.CheckoutAction.PaymentRequired
		metadata["initial_payment_amount_minor"] = output.CheckoutAction.PaymentAmountMinor
	}

	// Allocation-backed amounts are present when populateAllocationOutput ran.
	if output.ExpiresAt != nil {
		metadata["approved_financed_amount_minor"] = output.ApprovedFinancedAmountMinor
		metadata["upfront_amount_minor"] = output.UpfrontAmountMinor
		metadata["available_credit_minor"] = output.AvailableCreditMinor
		metadata["expires_at"] = *output.ExpiresAt
	} else if decision.ExpiresAt != nil {
		metadata["expires_at"] = *decision.ExpiresAt
	}

	changes := []audit.FieldChange{
		{
			Field:  "decision.outcome",
			Before: nil,
			After:  decision.Outcome,
		},
	}

	if offer := output.InstallmentOffer; offer != nil {
		if err := appendInstallmentOfferAuditMetadata(metadata, offer); err != nil {
			return audit.Event{}, fmt.Errorf("%s: build installment offer audit metadata: %w", op, err)
		}

		changes = append(changes,
			audit.FieldChange{
				Field:  "decision.approved_financed_amount_minor",
				Before: nil,
				After:  output.ApprovedFinancedAmountMinor,
			},
			audit.FieldChange{
				Field:  "decision.upfront_amount_minor",
				Before: nil,
				After:  output.UpfrontAmountMinor,
			},
			audit.FieldChange{
				Field:  "installments.status",
				Before: nil,
				After:  model.OfferedInstallmentStatus,
			},
		)
	}

	auditOutcome := audit.SucceededOutcome
	if decision.Outcome == model.DeniedDecisionOutcome {
		auditOutcome = audit.DeniedOutcome
	}

	event := audit.NewEvent(
		eventID,
		correlationID,
		nil,
		traceID,
		now,
		audit.EventData{
			Actor: audit.Actor{
				Type: audit.CustomerActorType,
				ID:   &actorID,
			},
			Action: checkoutDecisionAuditAction,
			Resource: audit.Resource{
				Type: checkoutDecisionResourceType,
				ID:   decision.ID.String(),
			},
			Outcome:    auditOutcome,
			ReasonCode: firstDecisionReasonCode(reasonCodes),
			Changes:    changes,
			Metadata:   metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate checkout decision audit event: %w", op, err)
	}

	return event, nil
}

func appendInstallmentOfferAuditMetadata(
	metadata map[string]any,
	offer *InstallmentOffer,
) error {
	const op = "decisionuc.appendInstallmentOfferAuditMetadata"

	installments := offer.Installments
	ids := make([]string, 0, len(installments))
	var (
		totalPrincipal uint64
		totalUpfront   uint64
		totalFee       uint64
	)

	for _, row := range installments {
		ids = append(ids, row.ID.String())

		var err error
		totalPrincipal, err = model.AddAmountMinor(totalPrincipal, row.PrincipalAmountMinor)
		if err != nil {
			return fmt.Errorf("%s: sum installment principal: %w", op, err)
		}

		totalUpfront, err = model.AddAmountMinor(totalUpfront, row.UpfrontAmountMinor)
		if err != nil {
			return fmt.Errorf("%s: sum installment upfront: %w", op, err)
		}
	}

	for _, charge := range offer.InstallmentCharges {
		var err error
		totalFee, err = model.AddAmountMinor(totalFee, charge.AmountMinor)
		if err != nil {
			return fmt.Errorf("%s: sum installment charges: %w", op, err)
		}
	}

	metadata["installment_count"] = offer.InstallmentCount
	metadata["installment_ids"] = ids
	metadata["payable_now_amount_minor"] = offer.PayableNowAmountMinor
	metadata["total_payable_amount_minor"] = offer.TotalPayableAmountMinor
	metadata["total_principal_amount_minor"] = totalPrincipal
	metadata["total_upfront_amount_minor"] = totalUpfront
	metadata["total_fee_amount_minor"] = totalFee

	if len(installments) == 0 {
		return nil
	}

	first := installments[0]
	last := installments[len(installments)-1]
	metadata["first_installment_id"] = first.ID.String()
	metadata["first_installment_due_at"] = first.DueAt
	metadata["last_installment_due_at"] = last.DueAt
	for _, installment := range installments[1:] {
		if installment.Status != first.Status {
			return fmt.Errorf("%s: installment offer contains mixed statuses: %q and %q", op, first.Status, installment.Status)
		}
	}

	metadata["offered_installment_status"] = first.Status

	return nil
}

func firstDecisionReasonCode(reasonCodes []string) *string {
	if len(reasonCodes) == 0 {
		return nil
	}

	reasonCode := reasonCodes[0]
	return &reasonCode
}
