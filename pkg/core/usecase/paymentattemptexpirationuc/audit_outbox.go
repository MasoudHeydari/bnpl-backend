package paymentattemptexpirationuc

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
	auditActionPaymentAttemptExpireLocalSessionCreation = "payment_attempt.expire_local_session_creation"
	auditActionPaymentAttemptCancelSessionCreationRetry = "payment_attempt.cancel_session_creation_retry_exhausted"
	auditReasonLocalSessionCreationWindowExpired        = "local_session_creation_window_expired"
	auditReasonSessionCreationRetryExhausted            = model.SessionCreationRetryExhaustedFailureCode
	auditServicePaymentAttemptExpirationTask            = "payment-attempt-expiration-task"
)

func (uc *UseCase) insertAuditOutbox(
	ctx context.Context,
	q repo.OutboxTxQueryer,
	event audit.Event,
	now time.Time,
) error {
	const op = "paymentattemptexpirationuc.UseCase.insertAuditOutbox"

	if err := audit.ValidateEvent(event); err != nil {
		return fmt.Errorf("%s: validate payment attempt expiration audit event: %w", op, err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal payment attempt expiration audit event: %w", op, err)
	}

	aggregateType := event.Data.Resource.Type
	_, err = q.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    event.Data.Resource.ID,
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert payment attempt expiration audit outbox message: %w", op, err)
	}

	return nil
}

func (uc *UseCase) newExpiredAuditEvent(
	before model.PaymentAttempt,
	after model.PaymentAttempt,
	now time.Time,
) audit.Event {
	service := auditServicePaymentAttemptExpirationTask
	reason := auditReasonLocalSessionCreationWindowExpired
	metadata := map[string]any{
		"payment_attempt_id":    after.ID.String(),
		"contract_id":           after.ContractID.String(),
		"installment_id":        after.InstallmentID.String(),
		"payment_purpose":       after.Purpose,
		"payment_attempt_state": after.State,
		"expired_at":            optionalTime(after.ExpiredAt),
		"financial_outcome_resolved": after.FinancialOutcomeResolved,
		"reason_codes":          []string{reason},
		"initiator":             "bnpl_system",
	}

	if before.ReconciliationReason != nil {
		metadata["prior_reconciliation_reason"] = *before.ReconciliationReason
	}

	return audit.NewEvent(
		uc.idGenerator.Generate(),
		"",
		nil,
		"",
		now,
		audit.EventData{
			Actor:      audit.Actor{Type: audit.SystemActorType, Service: &service},
			Action:     auditActionPaymentAttemptExpireLocalSessionCreation,
			Resource:   audit.Resource{Type: "payment_attempt", ID: after.ID.String()},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reason,
			Changes: []audit.FieldChange{
				{Field: "payment_attempt.state", Before: before.State, After: after.State},
				{Field: "payment_attempt.expired_at", Before: optionalTime(before.ExpiredAt), After: optionalTime(after.ExpiredAt)},
			},
			Metadata: metadata,
		},
	)
}

func (uc *UseCase) newRetryExhaustedCancelAuditEvent(
	before model.PaymentAttempt,
	after model.PaymentAttempt,
	now time.Time,
) audit.Event {
	service := auditServicePaymentAttemptExpirationTask
	reason := auditReasonSessionCreationRetryExhausted
	metadata := map[string]any{
		"payment_attempt_id":                     after.ID.String(),
		"contract_id":                            after.ContractID.String(),
		"installment_id":                         after.InstallmentID.String(),
		"payment_purpose":                        after.Purpose,
		"payment_attempt_state":                  after.State,
		"financial_session_create_attempt_count": after.FinancialSessionCreateAttemptCount,
		"failure_code":                           model.SessionCreationRetryExhaustedFailureCode,
		"financial_outcome_resolved":             after.FinancialOutcomeResolved,
		"reason_codes":                           []string{reason},
		"initiator":                              "bnpl_system",
	}

	return audit.NewEvent(
		uc.idGenerator.Generate(),
		"",
		nil,
		"",
		now,
		audit.EventData{
			Actor:      audit.Actor{Type: audit.SystemActorType, Service: &service},
			Action:     auditActionPaymentAttemptCancelSessionCreationRetry,
			Resource:   audit.Resource{Type: "payment_attempt", ID: after.ID.String()},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reason,
			Changes: []audit.FieldChange{
				{Field: "payment_attempt.state", Before: before.State, After: after.State},
				{Field: "payment_attempt.failure_code", Before: before.FailureCode, After: after.FailureCode},
			},
			Metadata: metadata,
		},
	)
}

func optionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}

	return value.UTC()
}
