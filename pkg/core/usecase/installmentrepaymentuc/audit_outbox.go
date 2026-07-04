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
	cancelSessionCreationRetryExhaustedAuditAction = "payment_attempt.cancel_session_creation_retry_exhausted"
	paymentAttemptAuditResourceType              = "payment_attempt"
	paymentAttemptCreatedReasonCode              = "installment_repayment_payment_attempt_created"
	sessionCreationRetryExhaustedReasonCode      = model.SessionCreationRetryExhaustedFailureCode
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

func (uc *UseCase) insertSessionCreationRetryExhaustedAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	previous model.PaymentAttempt,
	canceled model.PaymentAttempt,
	now time.Time,
) error {
	const op = "installmentrepaymentuc.UseCase.insertSessionCreationRetryExhaustedAuditOutbox"

	correlationID, traceID, err := audit.EnvelopeIDsFromContext(ctx)
	if err != nil {
		return fmt.Errorf("%s: create session-creation cancellation audit envelope ids: %w", op, err)
	}

	reasonCode := sessionCreationRetryExhaustedReasonCode
	previousState := previous.State
	actorService := "installment-repayment"
	metadata := map[string]any{
		"payment_attempt_id":                     canceled.ID.String(),
		"contract_id":                            canceled.ContractID.String(),
		"installment_id":                         canceled.InstallmentID.String(),
		"payment_purpose":                        canceled.Purpose,
		"financial_session_create_attempt_count": canceled.FinancialSessionCreateAttemptCount,
		"failure_code":                           model.SessionCreationRetryExhaustedFailureCode,
		"financial_outcome_resolved":             canceled.FinancialOutcomeResolved,
	}

	event := audit.NewEvent(
		uc.idGenerator.Generate(),
		correlationID,
		nil,
		traceID,
		now,
		audit.EventData{
			Actor:  audit.Actor{Type: audit.SystemActorType, Service: &actorService},
			Action: cancelSessionCreationRetryExhaustedAuditAction,
			Resource: audit.Resource{
				Type: paymentAttemptAuditResourceType,
				ID:   canceled.ID.String(),
			},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reasonCode,
			Changes: []audit.FieldChange{
				{
					Field:  "payment_attempt.state",
					Before: previousState,
					After:  canceled.State,
				},
				{
					Field:  "payment_attempt.failure_code",
					Before: previous.FailureCode,
					After:  canceled.FailureCode,
				},
			},
			Metadata: metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return fmt.Errorf("%s: validate session-creation cancellation audit event: %w", op, err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal session-creation cancellation audit event: %w", op, err)
	}

	aggregateType := paymentAttemptAuditResourceType
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    canceled.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert session-creation cancellation audit outbox message: %w", op, err)
	}

	return nil
}
