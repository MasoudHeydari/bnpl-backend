package installmentrepaymentuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestBuildCreatePaymentAttemptAuditEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 3, 14, 0, 0, 0, time.UTC)
	identitySubjectID := model.IdentitySubjectID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	chargeID := model.InstallmentChargeID(uuid.New())

	attempt := model.PaymentAttempt{
		ID:            attemptID,
		ContractID:    contractID,
		InstallmentID: installmentID,
		AmountMinor:   425_000,
		Currency:      model.IRRCurrency,
		Purpose:       model.InstallmentRepaymentFinancialCorePaymentPurpose,
		State:         model.CreatedPaymentAttemptState,
		Channel:       model.GatewayPaymentChannel,
		Items: []model.PaymentAttemptItem{
			{ItemType: model.PrincipalPaymentAttemptItemType, SourceID: installmentID.UUID(), AmountMinor: 400_000},
		},
	}
	installment := model.Installment{
		ID:               installmentID,
		DecisionID:       decisionID,
		ContractID:       &contractID,
		CustomerID:       customerID,
		InstallmentOrder: 2,
		Currency:         model.IRRCurrency,
		DueAt:            now.Add(24 * time.Hour),
		Status:           model.DueInstallmentStatus,
	}
	charges := []model.InstallmentCharge{
		{
			ID:            chargeID,
			InstallmentID: installmentID,
			ChargeType:    model.BNPLFeeInstallmentChargeType,
			ChargeStatus:  model.AppliedInstallmentChargeStatus,
			AmountMinor:   25_000,
			Currency:      model.IRRCurrency,
			PayerType:     model.CustomerInstallmentChargePayerType,
		},
	}

	event, err := buildCreatePaymentAttemptAuditEvent(
		"audit-event-id",
		"correlation-id",
		"trace-id",
		attempt,
		installmentRepaymentAuditFacts{
			IdentitySubjectID: identitySubjectID,
			CustomerID:        customerID,
			Installment:       installment,
			Charges:           charges,
		},
		now,
	)
	require.NoError(t, err)
	require.NoError(t, audit.ValidateEvent(event))

	assert.Equal(t, createInstallmentRepaymentAttemptAuditAction, event.Data.Action)
	assert.Equal(t, audit.CustomerActorType, event.Data.Actor.Type)
	require.NotNil(t, event.Data.Actor.ID)
	assert.Equal(t, identitySubjectID.String(), *event.Data.Actor.ID)
	assert.Equal(t, paymentAttemptAuditResourceType, event.Data.Resource.Type)
	assert.Equal(t, attemptID.String(), event.Data.Resource.ID)
	assert.Equal(t, audit.SucceededOutcome, event.Data.Outcome)
	require.NotNil(t, event.Data.ReasonCode)
	assert.Equal(t, paymentAttemptCreatedReasonCode, *event.Data.ReasonCode)
	assert.Equal(t, contractID.String(), event.Data.Metadata["contract_id"])
	assert.Equal(t, installmentID.String(), event.Data.Metadata["installment_id"])
	assert.Equal(t, customerID.String(), event.Data.Metadata["customer_id"])
	assert.Equal(t, decisionID.String(), event.Data.Metadata["decision_id"])
	assert.Equal(t, 1, event.Data.Metadata["customer_payable_charge_count"])
	assert.Equal(t, []string{chargeID.String()}, event.Data.Metadata["installment_charge_ids"])
	require.Len(t, event.Data.Changes, 1)
	assert.Equal(t, "payment_attempt.state", event.Data.Changes[0].Field)
	assert.Equal(t, model.CreatedPaymentAttemptState, event.Data.Changes[0].After)
}
