package policyassignmentrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.PolicyAssignment = (*Repo)(nil)

type Repo struct{}

func New() *Repo { return &Repo{} }

func (r *Repo) Conn(c repo.Conn) repo.PolicyAssignmentConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}
func (r *Repo) Tx(tx repo.Tx) repo.PolicyAssignmentTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct{ *postgres.Conn }

func (q connQueryer) GetActive(
	ctx context.Context,
	subjectType model.PolicySubjectType,
	subjectID string,
	family model.PolicyFamily,
	now time.Time,
) (model.PolicyAssignment, error) {
	return GetActive(
		ctx,
		q.Conn,
		subjectType,
		subjectID,
		family,
		now,
	)
}

type txQueryer struct{ *postgres.Tx }

func (q txQueryer) GetActive(
	ctx context.Context,
	subjectType model.PolicySubjectType,
	subjectID string,
	family model.PolicyFamily,
	now time.Time,
) (model.PolicyAssignment, error) {
	return GetActive(
		ctx,
		q.Tx,
		subjectType,
		subjectID,
		family,
		now,
	)
}
func (q txQueryer) LockActive(
	ctx context.Context,
	subjectType model.PolicySubjectType,
	subjectID string,
	family model.PolicyFamily,
	now time.Time,
) (model.PolicyAssignment, error) {
	return LockActive(
		ctx,
		q.Tx,
		subjectType,
		subjectID,
		family,
		now,
	)
}
func (q txQueryer) Create(
	ctx context.Context,
	subjectType model.PolicySubjectType,
	subjectID string,
	family model.PolicyFamily,
	policyID string,
	policyVersion string,
	reason string,
	source string,
	validFrom time.Time,
	createdBy string,
	createdAt time.Time,
	updatedAt time.Time,
) (model.PolicyAssignment, error) {
	return Create(
		ctx,
		q.Tx,
		subjectType,
		subjectID,
		family,
		policyID,
		policyVersion,
		reason,
		source,
		validFrom,
		createdBy,
		createdAt,
		updatedAt,
	)
}
func (q txQueryer) CloseActive(ctx context.Context, id model.PolicyAssignmentID, validUntil time.Time, updatedAt time.Time) (model.PolicyAssignment, error) {
	return CloseActive(
		ctx,
		q.Tx,
		id,
		validUntil,
		updatedAt,
	)
}
