package installmentlifecycleuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestBuildLifecycleAuditEvent_MarkDue(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	contractID := model.ContractID(uuid.New())
	before := lifecycleAuditInstallment(contractID, model.ScheduledInstallmentStatus, now)
	after := before
	after.Status = model.DueInstallmentStatus

	event, err := buildLifecycleAuditEvent("event-due", before, after, now)
	require.NoError(t, err)
	require.NoError(t, audit.ValidateEvent(event))
	require.Equal(t, markInstallmentDueAuditAction, event.Data.Action)
	require.Equal(t, before.ID.String(), event.Data.Resource.ID)
	require.Equal(t, before.Status, event.Data.Changes[0].Before)
	require.Equal(t, after.Status, event.Data.Changes[0].After)
	require.Equal(t, installmentDueReasonCode, *event.Data.ReasonCode)
	require.Equal(t, contractID.String(), event.Data.Metadata["contract_id"])
}

func TestBuildLifecycleAuditEvent_MarkOverdue(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	contractID := model.ContractID(uuid.New())
	before := lifecycleAuditInstallment(contractID, model.DueInstallmentStatus, now.Add(-5*24*time.Hour))
	after := before
	after.Status = model.OverdueInstallmentStatus

	event, err := buildLifecycleAuditEvent("event-overdue", before, after, now)
	require.NoError(t, err)
	require.NoError(t, audit.ValidateEvent(event))
	require.Equal(t, markInstallmentOverdueAuditAction, event.Data.Action)
	require.Equal(t, installmentOverdueReasonCode, *event.Data.ReasonCode)
	require.Equal(t, uint(5), event.Data.Metadata["grace_period_days"])
	require.Equal(t, before.OverdueEligibleAt(), event.Data.Metadata["overdue_eligible_at"])
}

func lifecycleAuditInstallment(
	contractID model.ContractID,
	status model.InstallmentStatus,
	dueAt time.Time,
) model.Installment {
	branchID := model.MerchantBranchProfileID(uuid.New())
	return model.Installment{
		ID:                      model.InstallmentID(uuid.New()),
		DecisionID:              model.DecisionID(uuid.New()),
		ContractID:              &contractID,
		CustomerID:              model.CustomerID(uuid.New()),
		MerchantProfileID:       model.MerchantProfileID(uuid.New()),
		MerchantBranchProfileID: &branchID,
		InstallmentOrder:        2,
		Currency:                model.IRRCurrency,
		PrincipalAmountMinor:    1_000_000,
		TotalAmountMinor:        1_100_000,
		DueAt:                   dueAt,
		GracePeriodDays:         5,
		Status:                  status,
	}
}

func TestBuildOfferExpiryAuditEventIncludesCanceledCharges(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	installmentID := model.InstallmentID(uuid.New())
	installmentRow := model.Installment{
		ID:                   installmentID,
		DecisionID:           model.DecisionID(uuid.New()),
		CustomerID:           model.CustomerID(uuid.New()),
		MerchantProfileID:    model.MerchantProfileID(uuid.New()),
		InstallmentOrder:     2,
		Currency:             model.IRRCurrency,
		PrincipalAmountMinor: 1_000_000,
		TotalAmountMinor:     1_050_000,
		DueAt:                now.Add(24 * time.Hour),
		Status:               model.ExpiredInstallmentStatus,
	}
	charges := []model.InstallmentCharge{
		{
			ID:            model.InstallmentChargeID(uuid.New()),
			InstallmentID: installmentID,
			ChargeStatus:  model.CanceledInstallmentChargeStatus,
			AmountMinor:   50_000,
			Currency:      model.IRRCurrency,
		},
	}

	event, err := buildOfferExpiryAuditEvent("audit-event-id", installmentRow, charges, now)
	require.NoError(t, err)
	require.Equal(t, expireInstallmentOfferAuditAction, event.Data.Action)
	require.Equal(t, installmentID.String(), event.Data.Resource.ID)
	require.Equal(t, installmentOfferExpiryReasonCode, *event.Data.ReasonCode)
	require.Len(t, event.Data.Changes, 2)
	require.Equal(t, 1, event.Data.Metadata["cancelled_charge_count"])
	require.Equal(t, uint64(50_000), event.Data.Metadata["cancelled_charge_amount_minor"])
}
