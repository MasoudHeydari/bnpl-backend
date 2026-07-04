package installmentrepaymentuc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const (
	createInstallmentRepaymentAttemptAuditAction = "payment_attempt.create_for_installment_repayment"
	paymentAttemptAuditResourceType              = "payment_attempt"
	paymentAttemptCreatedReasonCode              = "installment_repayment_payment_attempt_created"
)

type installmentRepaymentAuditFacts struct {
	ExternalCustomerID model.ExternalCustomerID
	CustomerID         model.CustomerID
	Installment        model.Installment
	Charges            []model.InstallmentCharge
}

func (uc *UseCase) insertCreatePaymentAttemptAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	attempt model.PaymentAttempt,
	facts installmentRepaymentAuditFacts,
	now time.Time,
) error {
	const op = "installmentrepaymentuc.UseCase.insertCreatePaymentAttemptAuditOutbox"

	correlationID, traceID, err := audit.EnvelopeIDsFromContext(ctx)
	if err != nil {
		return fmt.Errorf("%s: create installment repayment payment attempt audit envelope ids: %w", op, err)
	}

	event, err := buildCreatePaymentAttemptAuditEvent(
		uc.idGenerator.Generate(),
		correlationID,
		traceID,
		attempt,
		facts,
		now,
	)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal installment repayment payment attempt audit event: %w", op, err)
	}

	aggregateType := paymentAttemptAuditResourceType
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    attempt.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert installment repayment payment attempt audit outbox message: %w", op, err)
	}

	return nil
}

func buildCreatePaymentAttemptAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	attempt model.PaymentAttempt,
	facts installmentRepaymentAuditFacts,
	now time.Time,
) (audit.Event, error) {
	const op = "installmentrepaymentuc.buildCreatePaymentAttemptAuditEvent"

	actorID := facts.ExternalCustomerID.String()
	reasonCode := paymentAttemptCreatedReasonCode

	chargeIDs := make([]string, 0, len(facts.Charges))
	customerPayableChargeCount := 0
	for _, charge := range facts.Charges {
		chargeIDs = append(chargeIDs, charge.ID.String())
		if model.IsCustomerPayableCharge(charge) {
			customerPayableChargeCount++
		}
	}

	metadata := map[string]any{
		"payment_attempt_id":            attempt.ID.String(),
		"contract_id":                   attempt.ContractID.String(),
		"installment_id":                attempt.InstallmentID.String(),
		"customer_id":                   facts.CustomerID.String(),
		"identity_subject_id":           facts.ExternalCustomerID.String(),
		"decision_id":                   facts.Installment.DecisionID.String(),
		"installment_order":             facts.Installment.InstallmentOrder,
		"installment_status":            facts.Installment.Status,
		"installment_due_at":            facts.Installment.DueAt,
		"payment_attempt_state":         attempt.State,
		"payment_channel":               attempt.Channel,
		"payment_purpose":               attempt.Purpose,
		"amount_minor":                  attempt.AmountMinor,
		"currency":                      attempt.Currency,
		"payment_attempt_item_count":    len(attempt.Items),
		"installment_charge_count":      len(facts.Charges),
		"customer_payable_charge_count": customerPayableChargeCount,
		"installment_charge_ids":        chargeIDs,
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
			Action: createInstallmentRepaymentAttemptAuditAction,
			Resource: audit.Resource{
				Type: paymentAttemptAuditResourceType,
				ID:   attempt.ID.String(),
			},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reasonCode,
			Changes: []audit.FieldChange{
				{
					Field:  "payment_attempt.state",
					Before: nil,
					After:  attempt.State,
				},
			},
			Metadata: metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate installment repayment payment attempt audit event: %w", op, err)
	}

	return event, nil
}
