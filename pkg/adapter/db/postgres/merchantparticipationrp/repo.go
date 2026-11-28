package merchantparticipationrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Repo implements the Merchant participation repository.
type Repo struct{}

var _ repo.MerchantParticipation = (*Repo)(nil)

// New returns a stateless repository.
func New() *Repo { return &Repo{} }

func (r *Repo) Conn(c repo.Conn) repo.MerchantParticipationQueryer {
	return queryer[*postgres.Conn]{q: postgres.MustConn(c)}
}

func (r *Repo) Tx(tx repo.Tx) repo.MerchantParticipationTxQueryer {
	q := postgres.MustTx(tx)
	return txQueryer{queryer: queryer[*postgres.Tx]{q: q}, tx: q}
}

type queryer[Q postgres.Queryer] struct{ q Q }
type txQueryer struct {
	queryer[*postgres.Tx]
	tx *postgres.Tx
}

const columns = `request_id, application_id, external_merchant_id, merchant_profile_id,
 environment, progress_event_id::text AS progress_event_id, owner_version, delivery_status, attempt_count, next_attempt_at`

func (q queryer[Q]) Get(ctx context.Context, id model.MerchantParticipationRequestID) (model.MerchantParticipation, error) {
	const sql = `SELECT ` + columns + ` FROM merchant_participation_requests WHERE request_id=$1`
	return postgres.QueryOne[model.MerchantParticipation](
		ctx,
		q.q,
		"merchantparticipationrp.Get",
		sql,
		id,
	)
}

func (q txQueryer) Insert(ctx context.Context, p model.MerchantParticipation, now time.Time) error {
	const op = "merchantparticipationrp.Insert"
	const sql = `INSERT INTO merchant_participation_requests
 (request_id,application_id,external_merchant_id,merchant_profile_id,environment,progress_event_id,owner_version,delivery_status,attempt_count,next_attempt_at,created_at,updated_at)
 VALUES ($1,$2,$3,$4,$5,$6::text::uuid,$7,'pending',0,$8,$8,$8)`
	_, err := q.tx.Exec(
		ctx,
		sql,
		p.RequestID,
		p.ApplicationID,
		p.MerchantID,
		p.ProfileID,
		p.Environment,
		p.EventID,
		p.OwnerVersion,
		now,
	)
	if err != nil {
		return fmt.Errorf("%s: insert participation: %w", op, postgres.MapError(err))
	}

	return nil
}

// ClaimDue reserves one due delivery; a crashed worker's lease expires automatically.
func (q txQueryer) ClaimDue(ctx context.Context, now, leaseUntil time.Time) (model.MerchantParticipation, error) {
	const sql = `WITH candidate AS (
 SELECT request_id FROM merchant_participation_requests
 WHERE delivery_status='pending' AND next_attempt_at <= $1
 ORDER BY next_attempt_at, request_id LIMIT 1 FOR UPDATE SKIP LOCKED
 ) UPDATE merchant_participation_requests p
 SET attempt_count=p.attempt_count+1,next_attempt_at=$2,updated_at=$1
 WHERE p.request_id=(SELECT request_id FROM candidate) RETURNING ` + columns
	return postgres.QueryOne[model.MerchantParticipation](
		ctx,
		q.tx,
		"merchantparticipationrp.ClaimDue",
		sql,
		now,
		leaseUntil,
	)
}

// Finish never lets an expired worker overwrite a newer claim.
func (q txQueryer) Finish(ctx context.Context, r repo.MerchantProgressDeliveryResult) error {
	const op = "merchantparticipationrp.Finish"
	const sql = `UPDATE merchant_participation_requests SET delivery_status=$3,last_error=$4,
 next_attempt_at=$5,updated_at=$6 WHERE request_id=$1 AND attempt_count=$2 AND delivery_status='pending'`
	_, err := q.tx.Exec(
		ctx,
		sql,
		r.RequestID,
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
