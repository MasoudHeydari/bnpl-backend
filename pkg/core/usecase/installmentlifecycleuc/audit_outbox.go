package installmentlifecycleuc

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
	markInstallmentDueAuditAction     = "installment.mark_due"
	markInstallmentOverdueAuditAction = "installment.mark_overdue"
	installmentAuditResourceType      = "installment"
	installmentLifecycleService       = "installment-lifecycle-task"
	installmentDueReasonCode          = "installment_due_at_reached"
	installmentOverdueReasonCode      = "installment_grace_period_elapsed"
)

func (uc *UseCase) insertLifecycleAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	before model.Installment,
	after model.Installment,
	now time.Time,
) error {
	const op = "installmentlifecycleuc.UseCase.insertLifecycleAuditOutbox"

	eventID := uc.idGenerator.Generate()
	event, err := buildLifecycleAuditEvent(eventID, before, after, now)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal installment lifecycle audit event: %w", op, err)
	}

	aggregateType := installmentAuditResourceType
	if _, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    after.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	}); err != nil {
		return fmt.Errorf("%s: insert installment lifecycle audit outbox message: %w", op, err)
	}

	return nil
}

func buildLifecycleAuditEvent(
	eventID string,
	before model.Installment,
	after model.Installment,
	now time.Time,
) (audit.Event, error) {
	const op = "installmentlifecycleuc.buildLifecycleAuditEvent"

	action, reasonCode, err := lifecycleAuditAction(before.Status, after.Status)
	if err != nil {
		return audit.Event{}, err
	}

	service := installmentLifecycleService
	metadata := map[string]any{
		"installment_id":         after.ID.String(),
		"decision_id":            after.DecisionID.String(),
		"customer_id":            after.CustomerID.String(),
		"merchant_profile_id":    after.MerchantProfileID.String(),
		"installment_order":      after.InstallmentOrder,
		"due_at":                 after.DueAt,
		"grace_period_days":      after.GracePeriodDays,
		"overdue_eligible_at":    after.OverdueEligibleAt(),
		"principal_amount_minor": after.PrincipalAmountMinor,
		"total_amount_minor":     after.TotalAmountMinor,
		"currency":               after.Currency,
		"reason_codes":           []string{reasonCode},
	}
	if after.ContractID != nil {
		metadata["contract_id"] = after.ContractID.String()
	}

	if after.MerchantBranchProfileID != nil {
		metadata["merchant_branch_profile_id"] = after.MerchantBranchProfileID.String()
	}

	// Scheduled maintenance has no inbound request envelope. The generated event
	// identifier provides a stable non-empty correlation and trace fallback.
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
			Action: action,
			Resource: audit.Resource{
				Type: installmentAuditResourceType,
				ID:   after.ID.String(),
			},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reasonCode,
			Changes: []audit.FieldChange{
				{
					Field:  "installment.status",
					Before: before.Status,
					After:  after.Status,
				},
			},
			Metadata: metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate installment lifecycle audit event: %w", op, err)
	}

	return event, nil
}

func lifecycleAuditAction(
	before model.InstallmentStatus,
	after model.InstallmentStatus,
) (string, string, error) {
	const op = "installmentlifecycleuc.lifecycleAuditAction"

	switch {
	case before == model.ScheduledInstallmentStatus && after == model.DueInstallmentStatus:
		return markInstallmentDueAuditAction, installmentDueReasonCode, nil

	case before == model.DueInstallmentStatus && after == model.OverdueInstallmentStatus:
		return markInstallmentOverdueAuditAction, installmentOverdueReasonCode, nil

	default:
		return "", "", fmt.Errorf("%s: unsupported installment lifecycle audit transition %q -> %q", op, before, after)
	}
}

const (
	expireInstallmentOfferAuditAction = "installment_offer.expire"
	installmentOfferExpiryReasonCode  = "checkout_decision_allocation_expired"
)

func (uc *UseCase) insertOfferExpiryAuditOutbox(
	ctx context.Context,
	outboxQ repo.OutboxTxQueryer,
	expiredInstallment model.Installment,
	canceledCharges []model.InstallmentCharge,
	now time.Time,
) error {
	const op = "installmentlifecycleuc.UseCase.insertOfferExpiryAuditOutbox"

	eventID := uc.idGenerator.Generate()
	event, err := buildOfferExpiryAuditEvent(eventID, expiredInstallment, canceledCharges, now)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("%s: marshal installment offer expiry audit event: %w", op, err)
	}

	aggregateType := installmentAuditResourceType
	if _, err = outboxQ.Create(ctx, repo.CreateOutboxMessage{
		Destination:   uc.auditDestination,
		MessageKey:    expiredInstallment.ID.String(),
		EventType:     audit.EventType,
		AggregateType: &aggregateType,
		Payload:       payload,
		CreatedAt:     now,
	}); err != nil {
		return fmt.Errorf("%s: insert installment offer expiry audit outbox message: %w", op, err)
	}

	return nil
}

func buildOfferExpiryAuditEvent(
	eventID string,
	expiredInstallment model.Installment,
	canceledCharges []model.InstallmentCharge,
	now time.Time,
) (audit.Event, error) {
	const op = "installmentlifecycleuc.buildOfferExpiryAuditEvent"

	if expiredInstallment.Status != model.ExpiredInstallmentStatus {
		return audit.Event{}, fmt.Errorf(
			"%s: installment offer expiry audit requires expired installment status, got %q", op, expiredInstallment.Status,
		)
	}

	if expiredInstallment.ContractID != nil {
		return audit.Event{}, fmt.Errorf("%s: installment offer expiry audit requires unattached installment", op)
	}

	chargeIDs := make([]string, 0, len(canceledCharges))
	var canceledChargeAmountMinor uint64
	for _, charge := range canceledCharges {
		if charge.InstallmentID != expiredInstallment.ID {
			return audit.Event{}, fmt.Errorf("%s: canceled charge belongs to a different installment", op)
		}

		if charge.ChargeStatus != model.CanceledInstallmentChargeStatus {
			return audit.Event{}, fmt.Errorf(
				"%s: installment offer expiry audit requires canceled charge status, got %q", op, charge.ChargeStatus,
			)
		}

		chargeIDs = append(chargeIDs, charge.ID.String())
		canceledChargeAmountMinor += charge.AmountMinor
	}

	service := installmentLifecycleService
	reasonCode := installmentOfferExpiryReasonCode
	metadata := map[string]any{
		"installment_id":                expiredInstallment.ID.String(),
		"decision_id":                   expiredInstallment.DecisionID.String(),
		"customer_id":                   expiredInstallment.CustomerID.String(),
		"merchant_profile_id":           expiredInstallment.MerchantProfileID.String(),
		"installment_order":             expiredInstallment.InstallmentOrder,
		"due_at":                        expiredInstallment.DueAt,
		"principal_amount_minor":        expiredInstallment.PrincipalAmountMinor,
		"total_amount_minor":            expiredInstallment.TotalAmountMinor,
		"currency":                      expiredInstallment.Currency,
		"cancelled_charge_count":        len(canceledCharges),
		"cancelled_charge_ids":          chargeIDs,
		"cancelled_charge_amount_minor": canceledChargeAmountMinor,
		"reason_codes":                  []string{reasonCode},
	}
	if expiredInstallment.MerchantBranchProfileID != nil {
		metadata["merchant_branch_profile_id"] = expiredInstallment.MerchantBranchProfileID.String()
	}

	changes := []audit.FieldChange{
		{
			Field:  "installment.status",
			Before: model.OfferedInstallmentStatus,
			After:  model.ExpiredInstallmentStatus,
		},
	}
	if len(canceledCharges) > 0 {
		changes = append(changes, audit.FieldChange{
			Field:  "installment_charges.status",
			Before: model.OfferedInstallmentChargeStatus,
			After:  model.CanceledInstallmentChargeStatus,
		})
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
			Action: expireInstallmentOfferAuditAction,
			Resource: audit.Resource{
				Type: installmentAuditResourceType,
				ID:   expiredInstallment.ID.String(),
			},
			Outcome:    audit.SucceededOutcome,
			ReasonCode: &reasonCode,
			Changes:    changes,
			Metadata:   metadata,
		},
	)
	if err := audit.ValidateEvent(event); err != nil {
		return audit.Event{}, fmt.Errorf("%s: validate installment offer expiry audit event: %w", op, err)
	}

	return event, nil
}
