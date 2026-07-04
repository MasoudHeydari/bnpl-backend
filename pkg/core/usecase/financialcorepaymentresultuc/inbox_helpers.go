package financialcorepaymentresultuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/paymentapplication"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func (uc *UseCase) recordPayloadIdentityConflict(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	outboxQ repo.OutboxTxQueryer,
	inboxRow model.Inbox,
	input ProcessInput,
	purpose model.FinancialCorePaymentPurpose,
	now time.Time,
	output *ProcessOutput,
) error {
	incomingFingerprint, err := uc.paymentResultPayloadFingerprint(input, purpose)
	if err != nil {
		return err
	}

	_, _, err = inboxQ.RecordPayloadIdentityConflict(
		ctx,
		model.InboxPayloadConflictEvent{
			InboxID:                    inboxRow.ID,
			SourceService:              input.SourceService,
			EventID:                    input.EventID,
			ExistingPayloadFingerprint: inboxRow.PayloadFingerprint,
			IncomingPayloadFingerprint: incomingFingerprint,
			IncomingPayload:            input.RawPayload,
			FailureCode:                paymentapplication.EventIdentityPayloadMismatchFailureCode,
		},
		now,
	)
	if err != nil {
		return err
	}

	if err := uc.insertPayloadConflictAudit(ctx, outboxQ, input, inboxRow.ID, now); err != nil {
		return err
	}

	output.Result = ProcessResultPayloadConflictRecorded

	return nil
}

func mapInboxAcquireResult(result model.InboxAcquireResult, output *ProcessOutput) bool {
	switch result {
	case model.InboxAcquireAlreadyProcessed:
		output.Result = ProcessResultEventAlreadyProcessed
		return true

	case model.InboxAcquireReconciliationRequired:
		output.Result = ProcessResultReconciliationRequired
		return true

	case model.InboxAcquirePermanentlyFailed:
		output.Result = ProcessResultPermanentlyRejected
		return true

	default:
		return false
	}
}
