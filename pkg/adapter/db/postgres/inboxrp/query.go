package inboxrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const inboxColumns = `
	id,
	event_id,
	event_type,
	event_version,
	source_service,
	correlation_id,
	causation_id,
	payload,
	payload_fingerprint,
	consumer_name,
	subject_type,
	subject_id,
	external_reference_type,
	external_reference,
	status,
	received_at,
	processed_at,
	failed_at,
	failure_code,
	failure_message,
	created_at,
	updated_at
`

// GetBySourceAndEventID returns an inbox row by source service and external event id.
func GetBySourceAndEventID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	sourceService string,
	eventID string,
) (model.Inbox, error) {
	const query = `SELECT` + inboxColumns + `
FROM integration_inbox
WHERE source_service = $1
  AND event_id = $2
`

	return postgres.QueryOne[model.Inbox](
		ctx,
		q,
		"get inbox by source and event id",
		query,
		sourceService,
		eventID,
	)
}

// AcquireForProcessing inserts a received inbox row or locks an existing row in the same transaction.
func AcquireForProcessing[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	event model.InboxEvent,
	now time.Time,
) (model.Inbox, model.InboxAcquireResult, error) {
	const op = "inboxrp.AcquireForProcessing"

	if err := postgres.RequiredTimes(map[string]time.Time{
		"now": now,
	}); err != nil {
		return model.Inbox{}, "", err
	}

	if len(event.Payload) == 0 {
		event.Payload = []byte("{}")
	}

	const insertQuery = `
INSERT INTO integration_inbox (
	event_id,
	event_type,
	event_version,
	source_service,
	correlation_id,
	causation_id,
	payload,
	payload_fingerprint,
	consumer_name,
	subject_type,
	subject_id,
	external_reference_type,
	external_reference,
	status,
	received_at,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (source_service, event_id) DO NOTHING
RETURNING` + inboxColumns

	inserted, err := postgres.QueryOne[model.Inbox](
		ctx,
		q,
		"insert inbox row for processing",
		insertQuery,
		event.EventID,
		event.EventType,
		event.EventVersion,
		event.SourceService,
		event.CorrelationID,
		event.CausationID,
		event.Payload,
		event.PayloadFingerprint,
		string(event.ConsumerName),
		event.SubjectType,
		event.SubjectID,
		event.ExternalReferenceType,
		event.ExternalReference,
		model.ReceivedInboxStatus,
		now,
		now,
		now,
	)
	if err == nil {
		return inserted, model.InboxAcquireNew, nil
	}

	if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.Inbox{}, "", err
	}

	const lockQuery = `SELECT` + inboxColumns + `
FROM integration_inbox
WHERE source_service = $1
  AND event_id = $2
FOR UPDATE
`

	locked, lockErr := postgres.QueryOne[model.Inbox](
		ctx,
		q,
		"lock inbox row for processing",
		lockQuery,
		event.SourceService,
		event.EventID,
	)
	if lockErr != nil {
		return model.Inbox{}, "", lockErr
	}

	if locked.PayloadFingerprint != event.PayloadFingerprint {
		return locked, model.InboxAcquirePayloadMismatch, nil
	}

	switch locked.Status {
	case model.ProcessedInboxStatus:
		return locked, model.InboxAcquireAlreadyProcessed, nil

	case model.ReconciliationRequiredInboxStatus:
		return locked, model.InboxAcquireReconciliationRequired, nil

	case model.FailedInboxStatus:
		return locked, model.InboxAcquirePermanentlyFailed, nil

	case model.ReceivedInboxStatus:
		return locked, model.InboxAcquireNew, nil

	default:
		return model.Inbox{}, "", fmt.Errorf("%s: unexpected inbox status %q", op, locked.Status)
	}
}

// MarkProcessed marks an inbox row processed.
func MarkProcessed[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.InboxID,
	processedAt time.Time,
	updatedAt time.Time,
) (model.Inbox, error) {
	const query = `
UPDATE integration_inbox
SET
	status = $2,
	processed_at = $3,
	updated_at = $4
WHERE id = $1
  AND status = $5
RETURNING` + inboxColumns
	const op = "inboxrp.MarkProcessed"

	row, err := postgres.QueryOne[model.Inbox](
		ctx,
		q,
		"mark inbox processed",
		query,
		id,
		model.ProcessedInboxStatus,
		processedAt,
		updatedAt,
		model.ReceivedInboxStatus,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Inbox{}, cerr.Conflict(
				fmt.Errorf("%s: inbox row is not received", op),
				map[string]any{"inbox_id": id},
			)
		}

		return model.Inbox{}, err
	}

	return row, nil
}

// MarkReconciliationRequired marks an inbox row reconciliation_required.
func MarkReconciliationRequired[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.InboxID,
	failureCode *string,
	failureMessage *string,
	processedAt time.Time,
	updatedAt time.Time,
) (model.Inbox, error) {
	const query = `
UPDATE integration_inbox
SET
	status = $2,
	failure_code = $3,
	failure_message = $4,
	processed_at = $5,
	updated_at = $6
WHERE id = $1
  AND status = $7
RETURNING` + inboxColumns
	const op = "inboxrp.MarkReconciliationRequired"

	row, err := postgres.QueryOne[model.Inbox](
		ctx,
		q,
		"mark inbox reconciliation required",
		query,
		id,
		model.ReconciliationRequiredInboxStatus,
		failureCode,
		failureMessage,
		processedAt,
		updatedAt,
		model.ReceivedInboxStatus,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Inbox{}, cerr.Conflict(
				fmt.Errorf("%s: inbox row is not received", op),
				map[string]any{"inbox_id": id},
			)
		}

		return model.Inbox{}, err
	}

	return row, nil
}

// MarkFailed marks an inbox row failed.
func MarkFailed[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.InboxID,
	failureCode string,
	failureMessage string,
	failedAt time.Time,
	updatedAt time.Time,
) (model.Inbox, error) {
	const query = `
UPDATE integration_inbox
SET
	status = $2,
	failure_code = $3,
	failure_message = $4,
	failed_at = $5,
	updated_at = $6
WHERE id = $1
  AND status = $7
RETURNING` + inboxColumns
	const op = "inboxrp.MarkFailed"

	row, err := postgres.QueryOne[model.Inbox](
		ctx,
		q,
		"mark inbox failed",
		query,
		id,
		model.FailedInboxStatus,
		failureCode,
		failureMessage,
		failedAt,
		updatedAt,
		model.ReceivedInboxStatus,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Inbox{}, cerr.Conflict(
				fmt.Errorf("%s: inbox row is not received", op),
				map[string]any{"inbox_id": id},
			)
		}

		return model.Inbox{}, err
	}

	return row, nil
}
