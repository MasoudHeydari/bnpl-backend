package inboxrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const inboxConflictColumns = `
	id,
	inbox_id,
	source_service,
	event_id,
	existing_payload_fingerprint,
	incoming_payload_fingerprint,
	incoming_payload,
	failure_code,
	detected_at,
	created_at
`

// RecordPayloadIdentityConflict persists a payload identity conflict idempotently.
func RecordPayloadIdentityConflict[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	event model.InboxPayloadConflictEvent,
	now time.Time,
) (model.InboxPayloadConflict, bool, error) {
	const op = "inboxrp.RecordPayloadIdentityConflict"

	if len(event.IncomingPayload) == 0 {
		event.IncomingPayload = []byte("{}")
	}

	const insertQuery = `
INSERT INTO integration_inbox_conflicts (
	inbox_id,
	source_service,
	event_id,
	existing_payload_fingerprint,
	incoming_payload_fingerprint,
	incoming_payload,
	failure_code,
	detected_at,
	created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (source_service, event_id, incoming_payload_fingerprint) DO NOTHING
RETURNING` + inboxConflictColumns

	inserted, err := postgres.QueryOne[model.InboxPayloadConflict](
		ctx,
		q,
		"insert inbox payload identity conflict",
		insertQuery,
		event.InboxID,
		event.SourceService,
		event.EventID,
		event.ExistingPayloadFingerprint,
		event.IncomingPayloadFingerprint,
		event.IncomingPayload,
		event.FailureCode,
		now,
		now,
	)
	if err == nil {
		return inserted, true, nil
	}

	if !cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.InboxPayloadConflict{}, false, err
	}

	const selectQuery = `SELECT` + inboxConflictColumns + `
FROM integration_inbox_conflicts
WHERE source_service = $1
  AND event_id = $2
  AND incoming_payload_fingerprint = $3
`

	existing, selectErr := postgres.QueryOne[model.InboxPayloadConflict](
		ctx,
		q,
		"get inbox payload identity conflict",
		selectQuery,
		event.SourceService,
		event.EventID,
		event.IncomingPayloadFingerprint,
	)
	if selectErr != nil {
		return model.InboxPayloadConflict{}, false, fmt.Errorf("%s: resolve inbox payload identity conflict: %w", op, selectErr)
	}

	return existing, false, nil
}
