package merchantparticipationrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const merchantParticipationColumns = `
 r.request_id,
 r.application_id,
 r.external_merchant_id,
 e.merchant_profile_id,
 r.environment,
 d.progress_event_id::text AS progress_event_id,
 d.owner_version,
 e.declaration_revision,
 e.policy_id,
 e.policy_version,
 e.progress_status,
 e.issues,
 d.delivery_status,
 d.attempt_count,
 d.next_attempt_at
`

// Get returns the newest durably evaluated revision of a request.
func Get[Q postgres.Queryer](ctx context.Context, q Q, id model.MerchantParticipationRequestID) (model.MerchantParticipation, error) {
	const query = `SELECT` + merchantParticipationColumns + `
FROM merchant_participation_requests r
JOIN merchant_participation_evaluations e ON e.request_id = r.request_id
JOIN merchant_participation_progress_deliveries d
  ON d.request_id = e.request_id AND d.declaration_revision = e.declaration_revision
WHERE r.request_id = $1
ORDER BY d.owner_version DESC
LIMIT 1
`

	return postgres.QueryOne[model.MerchantParticipation](
		ctx,
		q,
		"merchantparticipationrp.Get",
		query,
		id,
	)
}

// Insert stores a new immutable evaluation and the matching report atomically.
func Insert(ctx context.Context, q *postgres.Tx, p model.MerchantParticipation, now time.Time) error {
	const op = "merchantparticipationrp.Insert"

	const identityQuery = `
INSERT INTO merchant_participation_requests (request_id, application_id, external_merchant_id, environment, created_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (request_id) DO NOTHING
`
	_, err := q.Exec(
		ctx,
		identityQuery,
		p.RequestID,
		p.ApplicationID,
		p.MerchantID,
		p.Environment,
		now,
	)
	if err != nil {
		return fmt.Errorf("%s: insert identity: %w", op, postgres.MapError(err))
	}

	const evaluationQuery = `
INSERT INTO merchant_participation_evaluations (
 request_id, declaration_revision, external_merchant_id, merchant_profile_id,
 policy_id, policy_version, progress_status, issues, created_at
)
SELECT request_id, $5, external_merchant_id, $6, $7, $8, $9, $10, $11
FROM merchant_participation_requests
WHERE request_id = $1 AND application_id = $2 AND external_merchant_id = $3 AND environment = $4
`
	result, err := q.Exec(
		ctx,
		evaluationQuery,
		p.RequestID,
		p.ApplicationID,
		p.MerchantID,
		p.Environment,
		p.DeclarationRevision,
		p.ProfileID,
		p.PolicyID,
		p.PolicyVersion,
		p.ProgressStatus,
		p.Issues,
		now,
	)
	if err != nil {
		return fmt.Errorf("%s: insert evaluation: %w", op, postgres.MapError(err))
	}

	if result != 1 {
		return fmt.Errorf("%s: immutable participation identity differs", op)
	}

	const deliveryQuery = `
INSERT INTO merchant_participation_progress_deliveries (
 progress_event_id, request_id, declaration_revision, owner_version,
 delivery_status, attempt_count, next_attempt_at, created_at, updated_at
)
VALUES ($1::text::uuid, $2, $3, $4, 'pending', 0, $5, $5, $5)
`
	_, err = q.Exec(
		ctx,
		deliveryQuery,
		p.EventID,
		p.RequestID,
		p.DeclarationRevision,
		p.OwnerVersion,
		now,
	)
	if err != nil {
		return fmt.Errorf("%s: insert progress delivery: %w", op, postgres.MapError(err))
	}

	return nil
}

// ClaimDue reserves one due delivery; a crashed worker's lease expires automatically.
func ClaimDue(ctx context.Context, q *postgres.Tx, now, leaseUntil time.Time) (model.MerchantParticipation, error) {
	const query = `
WITH candidate AS (
 SELECT progress_event_id
 FROM merchant_participation_progress_deliveries
 WHERE delivery_status = 'pending'
   AND next_attempt_at <= $1
 ORDER BY next_attempt_at, request_id, owner_version
 LIMIT 1
 FOR UPDATE SKIP LOCKED
), claimed AS (
 UPDATE merchant_participation_progress_deliveries p
 SET attempt_count = p.attempt_count + 1,
     next_attempt_at = $2,
     updated_at = $1
 WHERE p.progress_event_id = (SELECT progress_event_id FROM candidate)
 RETURNING p.*
)
SELECT` + merchantParticipationColumns + `
FROM claimed d
JOIN merchant_participation_requests r ON r.request_id = d.request_id
JOIN merchant_participation_evaluations e
  ON e.request_id = d.request_id AND e.declaration_revision = d.declaration_revision
`

	return postgres.QueryOne[model.MerchantParticipation](
		ctx,
		q,
		"merchantparticipationrp.ClaimDue",
		query,
		now,
		leaseUntil,
	)
}

// Finish never lets an expired worker overwrite a newer claim.
func Finish(ctx context.Context, q *postgres.Tx, r repo.MerchantProgressDeliveryResult) error {
	const op = "merchantparticipationrp.Finish"

	const query = `
UPDATE merchant_participation_progress_deliveries
SET delivery_status = $4,
    last_error = $5,
    next_attempt_at = $6,
    updated_at = $7
WHERE request_id = $1
  AND declaration_revision = $2
  AND attempt_count = $3
  AND delivery_status = 'pending'
`
	_, err := q.Exec(
		ctx,
		query,
		r.RequestID,
		r.DeclarationRevision,
		r.AttemptCount,
		r.Status,
		r.Reason,
		r.NextAttemptAt,
		r.Now,
	)
	if err != nil {
		return fmt.Errorf("%s: finish delivery: %w", op, postgres.MapError(err))
	}

	return nil
}
