package financialcorepaymentsessionuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/event/paymentapplication"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func mapInboxAcquireResult(result model.InboxAcquireResult, output *ProcessOutput) bool {
	switch result {
	case model.InboxAcquireAlreadyProcessed:
		output.Result = ProcessResultEventAlreadyProcessed
		return true

	case model.InboxAcquireReconciliationRequired:
		output.Result = ProcessResultSessionIdentityConflict
		return true

	case model.InboxAcquirePermanentlyFailed:
		output.Result = ProcessResultPermanentlyRejected
		return true

	default:
		return false
	}
}

func (uc *UseCase) recordPayloadIdentityConflict(
	ctx context.Context,
	inboxQ repo.InboxTxQueryer,
	inboxRow model.Inbox,
	input ProcessInput,
	now time.Time,
	output *ProcessOutput,
) error {
	incomingFingerprint, err := uc.paymentSessionPayloadFingerprint(input)
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

	output.Result = ProcessResultPayloadConflictRecorded

	return nil
}
