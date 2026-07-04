package contractuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestBuildExpirePaymentPendingAuditEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		8,
		5,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	attemptID := model.PaymentAttemptID(uuid.New())
	previousAttemptState := model.SubmittedPaymentAttemptState
	currentAttemptState := model.ManualReviewRequiredPaymentAttemptState
	reason := "checkout_contract_expired_before_terminal_payment_result"
	branchID := model.MerchantBranchProfileID(uuid.New())

	event, err := buildExpirePaymentPendingAuditEvent(
		"audit-event-id",
		expirePaymentPendingAuditFacts{
			Contract: model.Contract{
				ID:                      model.ContractID(uuid.New()),
				DecisionID:              model.DecisionID(uuid.New()),
				CustomerID:              model.CustomerID(uuid.New()),
				MerchantProfileID:       model.MerchantProfileID(uuid.New()),
				MerchantBranchProfileID: &branchID,
				Status:                  model.PaymentPendingContractStatus,
				ExpiresAt:               now.Add(-time.Minute),
			},
			ExpiredInstallmentCount:        3,
			CanceledChargeCount:            2,
			PaymentAttemptID:               &attemptID,
			PreviousPaymentAttemptState:    &previousAttemptState,
			CurrentPaymentAttemptState:     &currentAttemptState,
			PaymentAttemptTransitionReason: &reason,
		},
		now,
	)

	require.NoError(t, err)
	require.Equal(t, audit.EventType, event.EventType)
	require.Equal(t, expirePaymentPendingContractAuditAction, event.Data.Action)
	require.Equal(t, audit.SystemActorType, event.Data.Actor.Type)
	require.Equal(t, contractAuditResourceType, event.Data.Resource.Type)
	require.Equal(t, paymentPendingContractExpiredReasonCode, *event.Data.ReasonCode)
	require.Len(t, event.Data.Changes, 4)
	require.Equal(t, "contract.status", event.Data.Changes[0].Field)
	require.Equal(t, "installments.status", event.Data.Changes[1].Field)
	require.Equal(t, "installment_charges.status", event.Data.Changes[2].Field)
	require.Equal(t, "payment_attempt.state", event.Data.Changes[3].Field)
	require.Equal(t, previousAttemptState, event.Data.Changes[3].Before)
	require.Equal(t, currentAttemptState, event.Data.Changes[3].After)
	require.Equal(t, int64(3), event.Data.Metadata["expired_installment_count"])
	require.Equal(t, int64(2), event.Data.Metadata["cancelled_charge_count"])
	require.Equal(t, attemptID.String(), event.Data.Metadata["payment_attempt_id"])
	require.Equal(t, previousAttemptState, event.Data.Metadata["previous_payment_attempt_state"])
	require.Equal(t, currentAttemptState, event.Data.Metadata["current_payment_attempt_state"])
	require.Equal(t, reason, event.Data.Metadata["payment_attempt_transition_reason"])
}
