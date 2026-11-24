package policyassignmentrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.CreditOwnerPolicyAssignmentQueryer = connQueryer{}
var _ repo.CreditOwnerPolicyAssignmentQueryer = txQueryer{}

func (q connQueryer) GetActiveForCreditOwner(ctx context.Context, customer model.ExternalCustomerID, family model.PolicyFamily, now time.Time) (model.PolicyAssignment, error) {
	return getActiveForOwner(
		ctx,
		q.Conn,
		customer,
		family,
		now,
	)
}
func (q txQueryer) GetActiveForCreditOwner(ctx context.Context, customer model.ExternalCustomerID, family model.PolicyFamily, now time.Time) (model.PolicyAssignment, error) {
	return getActiveForOwner(
		ctx,
		q.Tx,
		customer,
		family,
		now,
	)
}
func getActiveForOwner[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customer model.ExternalCustomerID,
	family model.PolicyFamily,
	now time.Time,
) (model.PolicyAssignment, error) {
	const query = `SELECT pa.id, pa.subject_type, pa.subject_id, pa.policy_family, pa.policy_id, pa.policy_version,
        pa.reason, pa.source, pa.valid_from, pa.valid_until, pa.created_by, pa.created_at, pa.updated_at
    FROM customer_profiles cp
    JOIN policy_assignments pa ON pa.subject_type='credit_owner' AND pa.subject_id=cp.credit_owner_id::text
    WHERE cp.external_customer_id=$1 AND pa.policy_family=$2 AND pa.valid_from<=$3
      AND (pa.valid_until IS NULL OR pa.valid_until>$3)
    ORDER BY pa.valid_from DESC, pa.created_at DESC LIMIT 1`
	return postgres.QueryOne[model.PolicyAssignment](
		ctx,
		q,
		"get legal person policy assignment",
		query,
		customer,
		family,
		now,
	)
}
