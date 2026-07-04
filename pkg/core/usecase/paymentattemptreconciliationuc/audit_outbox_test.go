package paymentattemptreconciliationuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestNewPaymentAttemptReconciliationAuditEvent_Valid(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	reason := model.PaymentResultTimeoutPaymentReconciliationReason
	attempt := model.PaymentAttempt{
		ID: model.PaymentAttemptID(uuid.New()), ContractID: model.ContractID(uuid.New()), InstallmentID: model.InstallmentID(uuid.New()),
		AmountMinor: 1000, Currency: model.IRRCurrency,
		Purpose:              model.InstallmentRepaymentFinancialCorePaymentPurpose,
		Channel:              model.GatewayPaymentChannel,
		State:                model.ReconciliationRequiredPaymentAttemptState,
		ReconciliationReason: &reason,
	}

	event := newPaymentAttemptReconciliationAuditEvent(
		"audit-event-id",
		auditActionPaymentAttemptRequireReconciliation,
		string(reason),
		audit.SucceededOutcome,
		attempt,
		now,
		[]audit.FieldChange{{
			Field:  "payment_attempt.state",
			Before: model.SubmittedPaymentAttemptState,
			After:  model.ReconciliationRequiredPaymentAttemptState,
		}},
		nil,
	)

	require.NoError(t, audit.ValidateEvent(event))
	require.Equal(t, attempt.ID.String(), event.Data.Resource.ID)
	require.Equal(t, auditActionPaymentAttemptRequireReconciliation, event.Data.Action)
	require.NotNil(t, event.Data.ReasonCode)
	require.Equal(t, string(reason), *event.Data.ReasonCode)
	require.Equal(t, []string{string(reason)}, event.Data.Metadata["reason_codes"])
}
