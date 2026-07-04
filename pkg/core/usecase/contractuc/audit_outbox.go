package contractuc

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
	createPaymentPendingContractAuditAction = "contract.create_payment_pending"
	contractAuditResourceType               = "contract"
	paymentPendingContractCreatedReasonCode = "checkout_contract_created_for_first_payment"

	expirePaymentPendingContractAuditAction = "contract.expire_payment_pending"
	paymentPendingContractExpiryService     = "payment-pending-contract-expiration-task"
	paymentPendingContractExpiredReasonCode = "payment_pending_contract_expired"

	cancelContractAuditAction         = "contract.cancellation_requested"
	contractCancellationStartedReason = "contract_cancellation_started"
)

type createPaymentPendingAuditFacts struct {
	ExternalCustomerID model.ExternalCustomerID
	Contract           model.Contract
	Installments       []model.Installment
	InstallmentCharges []model.InstallmentCharge
	PaymentAttempt     model.PaymentAttempt
}

type expirePaymentPendingAuditFacts struct {
	Contract                       model.Contract
	ExpiredInstallmentCount        int64
	CanceledChargeCount            int64
	PaymentAttemptID               *model.PaymentAttemptID
	PreviousPaymentAttemptState    *model.PaymentAttemptState
	CurrentPaymentAttemptState     *model.PaymentAttemptState
	PaymentAttemptTransitionReason *string
}

func (uc *UseCase) insertCreatePaymentPendingAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	facts createPaymentPendingAuditFacts,
	now time.Time,
) error {
	const op = "contractuc.UseCase.insertCreatePaymentPendingAuditOutbox"

	correlationID, traceID, err := audit.EnvelopeIDsFromContext(ctx)
	if err != nil {
		return fmt.Errorf("%s: create payment pending contract audit envelope ids: %w", op, err)
	}

	event, err := buildCreatePaymentPendingAuditEvent(
		uc.idGenerator.Generate(),
		correlationID,
		traceID,
		facts,
		now,
	)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal payment pending contract audit event: %w", op, err)
	}

	aggregateType := contractAuditResourceType
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    facts.Contract.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("%s: insert payment pending contract audit outbox message: %w", op, err)
	}

	return nil
}

func buildCreatePaymentPendingAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	facts createPaymentPendingAuditFacts,
	now time.Time,
) (audit.Event, error) {
	const op = "contractuc.buildCreatePaymentPendingAuditEvent"

	actorID := facts.ExternalCustomerID.String()
	reasonCode := paymentPendingContractCreatedReasonCode

	installmentIDs := make([]string, 0, len(facts.Installments))
	for _, installment := range facts.Installments {
		installmentIDs = append(installmentIDs, installment.ID.String())
	}

	chargeIDs := make([]string, 0, len(facts.InstallmentCharges))
	for _, charge := range facts.InstallmentCharges {
		chargeIDs = append(chargeIDs, charge.ID.String())
	}

	metadata := map[string]any{
		"contract_id":                      facts.Contract.ID.String(),
		"decision_id":                      facts.Contract.DecisionID.String(),
		"checkout_key":                     facts.Contract.CheckoutKey,
		"customer_id":                      facts.Contract.CustomerID.String(),
		"identity_subject_id":              facts.ExternalCustomerID.String(),
		"merchant_profile_id":              facts.Contract.MerchantProfileID.String(),
		"currency":                         facts.Contract.Currency,
		"requested_amount_minor":           facts.Contract.RequestedAmountMinor,
		"approved_financed_amount_minor":   facts.Contract.ApprovedFinancedAmountMinor,
		"upfront_amount_minor":             facts.Contract.UpfrontAmountMinor,
		"payable_now_amount_minor":         facts.Contract.PayableNowAmountMinor,
		"remaining_principal_amount_minor": facts.Contract.RemainingPrincipalAmountMinor,
		"installment_count":                facts.Contract.InstallmentCount,
		"installment_ids":                  installmentIDs,
		"installment_charge_ids":           chargeIDs,
		"payment_attempt_id":               facts.PaymentAttempt.ID.String(),
		"payment_attempt_state":            facts.PaymentAttempt.State,
		"payment_channel":                  facts.PaymentAttempt.Channel,
		"payment_purpose":                  facts.PaymentAttempt.Purpose,
		"expires_at":                       facts.Contract.ExpiresAt,
		"reason_codes":                     []string{reasonCode},
	}
	if facts.Contract.MerchantBranchProfileID != nil {
		metadata["merchant_branch_profile_id"] = facts.Contract.MerchantBranchProfileID.String()
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
			Action: createPaymentPendingContractAuditAction,
			Resource: audit.Resource{
				Type: contractAuditResourceType,
				ID:   facts.Contract.ID.String(),
			},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reasonCode,
			Changes: []audit.FieldChange{
				{Field: "contract.status", Before: nil, After: facts.Contract.Status},
				{Field: "installments.contract_id", Before: nil, After: facts.Contract.ID.String()},
				{Field: "payment_attempt.state", Before: nil, After: facts.PaymentAttempt.State},
			},
			Metadata: metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate payment pending contract audit event: %w", op, err)
	}

	return event, nil
}

func (uc *UseCase) insertExpirePaymentPendingAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	facts expirePaymentPendingAuditFacts,
	now time.Time,
) error {
	const op = "contractuc.UseCase.insertExpirePaymentPendingAuditOutbox"

	eventID := uc.idGenerator.Generate()
	event, err := buildExpirePaymentPendingAuditEvent(eventID, facts, now)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal payment pending contract expiry audit event: %w", op, err)
	}

	aggregateType := contractAuditResourceType
	if _, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    facts.Contract.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	}); err != nil {
		return fmt.Errorf("%s: insert payment pending contract expiry audit outbox message: %w", op, err)
	}

	return nil
}

func buildExpirePaymentPendingAuditEvent(
	eventID string,
	facts expirePaymentPendingAuditFacts,
	now time.Time,
) (audit.Event, error) {
	const op = "contractuc.buildExpirePaymentPendingAuditEvent"

	service := paymentPendingContractExpiryService
	reasonCode := paymentPendingContractExpiredReasonCode
	metadata := map[string]any{
		"contract_id":               facts.Contract.ID.String(),
		"decision_id":               facts.Contract.DecisionID.String(),
		"customer_id":               facts.Contract.CustomerID.String(),
		"merchant_profile_id":       facts.Contract.MerchantProfileID.String(),
		"contract_expires_at":       facts.Contract.ExpiresAt,
		"expired_at":                now,
		"previous_contract_status":  model.PaymentPendingContractStatus,
		"current_contract_status":   model.ExpiredContractStatus,
		"expired_installment_count": facts.ExpiredInstallmentCount,
		"cancelled_charge_count":    facts.CanceledChargeCount,
		"reason_codes":              []string{reasonCode},
	}
	if facts.Contract.MerchantBranchProfileID != nil {
		metadata["merchant_branch_profile_id"] = facts.Contract.MerchantBranchProfileID.String()
	}

	changes := []audit.FieldChange{
		{
			Field:  "contract.status",
			Before: model.PaymentPendingContractStatus,
			After:  model.ExpiredContractStatus,
		},
	}

	if facts.ExpiredInstallmentCount > 0 {
		changes = append(
			changes,
			audit.FieldChange{
				Field:  "installments.status",
				Before: model.OfferedInstallmentStatus,
				After:  model.ExpiredInstallmentStatus,
			},
		)
	}

	if facts.CanceledChargeCount > 0 {
		changes = append(
			changes,
			audit.FieldChange{
				Field:  "installment_charges.status",
				Before: model.OfferedInstallmentChargeStatus,
				After:  model.CanceledInstallmentChargeStatus,
			},
		)
	}

	if facts.PaymentAttemptID != nil && facts.PreviousPaymentAttemptState != nil && facts.CurrentPaymentAttemptState != nil {
		metadata["payment_attempt_id"] = facts.PaymentAttemptID.String()
		metadata["previous_payment_attempt_state"] = *facts.PreviousPaymentAttemptState
		metadata["current_payment_attempt_state"] = *facts.CurrentPaymentAttemptState
		if facts.PaymentAttemptTransitionReason != nil {
			metadata["payment_attempt_transition_reason"] = *facts.PaymentAttemptTransitionReason
		}

		changes = append(
			changes,
			audit.FieldChange{
				Field:  "payment_attempt.state",
				Before: *facts.PreviousPaymentAttemptState,
				After:  *facts.CurrentPaymentAttemptState,
			},
		)
	}

	event := audit.NewEvent(
		eventID,
		eventID,
		nil,
		eventID,
		now,
		audit.EventData{
			Actor: audit.Actor{
				Type:    audit.SystemActorType,
				Service: &service,
			},
			Action: expirePaymentPendingContractAuditAction,
			Resource: audit.Resource{
				Type: contractAuditResourceType,
				ID:   facts.Contract.ID.String(),
			},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reasonCode,
			Changes:    changes,
			Metadata:   metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate payment pending contract expiry audit event: %w", op, err)
	}

	return event, nil
}

const (
	attachCheckoutPaymentSessionAuditAction = "payment_attempt.attach_financial_payment_session"
	financialCoreAuditService               = "financial-core"
	financialPaymentSessionAttachedReason   = "financial_payment_session_attached"
	contractPaymentAttemptAuditResourceType = "payment_attempt"
)

type attachCheckoutPaymentSessionAuditFacts struct {
	Contract             model.Contract
	PaymentAttempt       model.PaymentAttempt
	PreviousAttemptState model.PaymentAttemptState
}

func (uc *UseCase) insertAttachCheckoutPaymentSessionAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	facts attachCheckoutPaymentSessionAuditFacts,
	now time.Time,
) error {
	const op = "contractuc.UseCase.insertAttachCheckoutPaymentSessionAuditOutbox"

	correlationID, traceID, err := audit.EnvelopeIDsFromContext(ctx)
	if err != nil {
		return fmt.Errorf("%s: checkout payment session audit envelope ids: %w", op, err)
	}

	reasonCode := financialPaymentSessionAttachedReason
	service := financialCoreAuditService
	metadata := map[string]any{
		"contract_id":                    facts.Contract.ID.String(),
		"decision_id":                    facts.Contract.DecisionID.String(),
		"customer_id":                    facts.Contract.CustomerID.String(),
		"merchant_profile_id":            facts.Contract.MerchantProfileID.String(),
		"payment_attempt_id":             facts.PaymentAttempt.ID.String(),
		"previous_payment_attempt_state": facts.PreviousAttemptState,
		"current_payment_attempt_state":  facts.PaymentAttempt.State,
		"payment_purpose":                facts.PaymentAttempt.Purpose,
		"payment_channel":                facts.PaymentAttempt.Channel,
		"amount_minor":                   facts.PaymentAttempt.AmountMinor,
		"currency":                       facts.PaymentAttempt.Currency,
		"reason_codes":                   []string{reasonCode},
	}
	if facts.Contract.MerchantBranchProfileID != nil {
		metadata["merchant_branch_profile_id"] = facts.Contract.MerchantBranchProfileID.String()
	}

	if facts.PaymentAttempt.FinancialPaymentIntentID != nil {
		metadata["financial_payment_intent_id"] = *facts.PaymentAttempt.FinancialPaymentIntentID
	}

	if facts.PaymentAttempt.FinancialPaymentAttemptID != nil {
		metadata["financial_payment_attempt_id"] = *facts.PaymentAttempt.FinancialPaymentAttemptID
	}

	if facts.PaymentAttempt.FinancialPaymentSessionID != nil {
		metadata["financial_payment_session_id"] = *facts.PaymentAttempt.FinancialPaymentSessionID
	}

	if facts.PaymentAttempt.FinancialPaymentExpiresAt != nil {
		metadata["financial_payment_session_expires_at"] = facts.PaymentAttempt.FinancialPaymentExpiresAt.UTC()
	}

	changes := []audit.FieldChange{
		{
			Field:  "payment_attempt.state",
			Before: facts.PreviousAttemptState,
			After:  facts.PaymentAttempt.State,
		},
	}
	if facts.PaymentAttempt.FinancialPaymentSessionID != nil {
		changes = append(changes, audit.FieldChange{
			Field:  "payment_attempt.financial_payment_session_id",
			Before: nil,
			After:  *facts.PaymentAttempt.FinancialPaymentSessionID,
		})
	}

	event := audit.NewEvent(
		uc.idGenerator.Generate(),
		correlationID,
		nil,
		traceID,
		now,
		audit.EventData{
			Actor: audit.Actor{
				Type:    audit.ServiceActorType,
				Service: &service,
			},
			Action: attachCheckoutPaymentSessionAuditAction,
			Resource: audit.Resource{
				Type: contractPaymentAttemptAuditResourceType,
				ID:   facts.PaymentAttempt.ID.String(),
			},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reasonCode,
			Changes:    changes,
			Metadata:   metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return fmt.Errorf("%s: validate checkout payment session audit event: %w", op, err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal checkout payment session audit event: %w", op, err)
	}

	aggregateType := contractPaymentAttemptAuditResourceType
	if _, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    facts.PaymentAttempt.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	}); err != nil {
		return fmt.Errorf("%s: insert checkout payment session audit outbox message: %w", op, err)
	}

	return nil
}

type cancelAuditFacts struct {
	ExternalCustomerID model.ExternalCustomerID
	Contract           model.Contract
	PreviousStatus     model.ContractStatus
	Reason             model.ContractCancellationReason
}

func (uc *UseCase) insertCancelAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	facts cancelAuditFacts,
	now time.Time,
) error {
	const op = "contractuc.UseCase.insertCancelAuditOutbox"

	correlationID, traceID, err := audit.EnvelopeIDsFromContext(ctx)
	if err != nil {
		return fmt.Errorf("%s: cancel contract audit envelope ids: %w", op, err)
	}

	event, err := buildCancelAuditEvent(
		uc.idGenerator.Generate(),
		correlationID,
		traceID,
		facts,
		now,
	)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal cancel contract audit event: %w", op, err)
	}

	aggregateType := contractAuditResourceType
	if _, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    facts.Contract.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	}); err != nil {
		return fmt.Errorf("%s: insert cancel contract audit outbox message: %w", op, err)
	}

	return nil
}

func buildCancelAuditEvent(
	eventID string,
	correlationID string,
	traceID string,
	facts cancelAuditFacts,
	now time.Time,
) (audit.Event, error) {
	const op = "contractuc.buildCancelAuditEvent"

	actorID := facts.ExternalCustomerID.String()
	reasonCode := contractCancellationStartedReason

	metadata := map[string]any{
		"contract_id":                      facts.Contract.ID.String(),
		"customer_id":                      facts.Contract.CustomerID.String(),
		"identity_subject_id":              facts.ExternalCustomerID.String(),
		"previous_status":                  facts.PreviousStatus,
		"new_status":                       facts.Contract.Status,
		"reason":                           facts.Reason.String(),
		"reason_codes":                     []string{reasonCode},
		"remaining_principal_amount_minor": facts.Contract.RemainingPrincipalAmountMinor,
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
			Action: cancelContractAuditAction,
			Resource: audit.Resource{
				Type: contractAuditResourceType,
				ID:   facts.Contract.ID.String(),
			},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reasonCode,
			Changes: []audit.FieldChange{
				{
					Field:  "contract.status",
					Before: facts.PreviousStatus,
					After:  facts.Contract.Status,
				},
			},
			Metadata: metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate cancel contract audit event: %w", op, err)
	}

	return event, nil
}

const (
	createActiveContractAuditAction = "contract.create_active"
	activeContractCreatedReasonCode = "checkout_contract_activated_without_initial_payment"
)

func (uc *UseCase) insertCreateActiveContractAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	externalCustomerID model.ExternalCustomerID,
	contractRow model.Contract,
	installments []model.Installment,
	now time.Time,
) error {
	const op = "contractuc.UseCase.insertCreateActiveContractAuditOutbox"

	correlationID, traceID, err := audit.EnvelopeIDsFromContext(ctx)
	if err != nil {
		return fmt.Errorf("%s: audit envelope ids: %w", op, err)
	}

	installmentIDs := make([]string, 0, len(installments))
	for _, installment := range installments {
		installmentIDs = append(installmentIDs, installment.ID.String())
	}

	actorID := externalCustomerID.String()
	reasonCode := activeContractCreatedReasonCode
	event := audit.NewEvent(
		uc.idGenerator.Generate(),
		correlationID,
		nil,
		traceID,
		now,
		audit.EventData{
			Actor:      audit.Actor{Type: audit.CustomerActorType, ID: &actorID},
			Action:     createActiveContractAuditAction,
			Resource:   audit.Resource{Type: contractAuditResourceType, ID: contractRow.ID.String()},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reasonCode,
			Changes: []audit.FieldChange{
				{
					Field:  "contract.status",
					Before: model.PaymentPendingContractStatus,
					After:  contractRow.Status,
				},
			},
			Metadata: map[string]any{
				"contract_id":                      contractRow.ID.String(),
				"decision_id":                      contractRow.DecisionID.String(),
				"checkout_key":                     contractRow.CheckoutKey,
				"customer_id":                      contractRow.CustomerID.String(),
				"identity_subject_id":              externalCustomerID.String(),
				"merchant_profile_id":              contractRow.MerchantProfileID.String(),
				"currency":                         contractRow.Currency,
				"requested_amount_minor":           contractRow.RequestedAmountMinor,
				"approved_financed_amount_minor":   contractRow.ApprovedFinancedAmountMinor,
				"upfront_amount_minor":             contractRow.UpfrontAmountMinor,
				"remaining_principal_amount_minor": contractRow.RemainingPrincipalAmountMinor,
				"installment_count":                contractRow.InstallmentCount,
				"installment_ids":                  installmentIDs,
				"reason_codes":                     []string{reasonCode},
			},
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return fmt.Errorf("%s: validate active contract audit event: %w", op, err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal active contract audit event: %w", op, err)
	}

	aggregateType := contractAuditResourceType
	_, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    contractRow.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	})
	return err
}
