package customerrestrictionrp

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.PersonCreditRestrictionReader = txQueryer{}
var _ repo.PersonCreditRestrictionReader = connQueryer{}

func (q txQueryer) ListActiveForCreditOwner(ctx context.Context, customer model.CustomerID) ([]model.CustomerRestriction, error) {
	return listActiveForCreditOwner(ctx, q.Tx, customer)
}
func (q connQueryer) ListActiveForCreditOwner(ctx context.Context, customer model.CustomerID) ([]model.CustomerRestriction, error) {
	return listActiveForCreditOwner(ctx, q.Conn, customer)
}
func listActiveForCreditOwner[Q postgres.Queryer](ctx context.Context, q Q, customer model.CustomerID) ([]model.CustomerRestriction, error) {
	const query = `SELECT ` + restrictionColumns + ` FROM customer_restrictions
    WHERE credit_owner_id=(SELECT credit_owner_id FROM customer_profiles WHERE id=$1)
      AND status=$2 ORDER BY applied_at,id`
	return postgres.QueryAll[model.CustomerRestriction](
		ctx,
		q,
		"list legal person credit restrictions",
		query,
		customer,
		model.ActiveCustomerRestrictionStatus,
	)
}
