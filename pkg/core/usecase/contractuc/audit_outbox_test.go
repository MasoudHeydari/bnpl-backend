package contractuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestBuildCreatePaymentPendingAuditEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		time.August,
		3,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	branchID := model.MerchantBranchProfileID(uuid.New())
	contractID := model.ContractID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	chargeID := model.InstallmentChargeID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())

	event, err := buildCreatePaymentPendingAuditEvent(
		"audit-event-1",
		"correlation-1",
		"trace-1",
		createPaymentPendingAuditFacts{
			ExternalCustomerID: externalCustomerID,
			Contract: model.Contract{
				ID:                            contractID,
				DecisionID:                    decisionID,
				CheckoutKey:                   "checkout-1",
				CustomerID:                    customerID,
				MerchantProfileID:             merchantProfileID,
				MerchantBranchProfileID:       &branchID,
				Currency:                      model.Currency("IRR"),
				RequestedAmountMinor:          1_200_000,
				ApprovedFinancedAmountMinor:   1_000_000,
				UpfrontAmountMinor:            200_000,
				InstallmentCount:              4,
				PayableNowAmountMinor:         450_000,
				RemainingPrincipalAmountMinor: 750_000,
				Status:                        model.PaymentPendingContractStatus,
				ExpiresAt:                     now.Add(30 * time.Minute),
			},
			Installments:       []model.Installment{{ID: installmentID}},
			InstallmentCharges: []model.InstallmentCharge{{ID: chargeID}},
			PaymentAttempt: model.PaymentAttempt{
				ID:      attemptID,
				State:   model.CreatedPaymentAttemptState,
				Channel: model.GatewayPaymentChannel,
				Purpose: model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
			},
		},
		now,
	)
	require.NoError(t, err)
	require.NoError(t, audit.ValidateEvent(event))
	require.Equal(t, createPaymentPendingContractAuditAction, event.Data.Action)
	require.Equal(t, contractAuditResourceType, event.Data.Resource.Type)
	require.Equal(t, contractID.String(), event.Data.Resource.ID)
	require.Equal(t, audit.CustomerActorType, event.Data.Actor.Type)
	require.NotNil(t, event.Data.Actor.ID)
	require.Equal(t, externalCustomerID.String(), *event.Data.Actor.ID)
	require.NotNil(t, event.Data.ReasonCode)
	require.Equal(t, paymentPendingContractCreatedReasonCode, *event.Data.ReasonCode)
	require.Equal(t, attemptID.String(), event.Data.Metadata["payment_attempt_id"])
	require.Equal(t, branchID.String(), event.Data.Metadata["merchant_branch_profile_id"])
	require.Len(t, event.Data.Changes, 3)
}
