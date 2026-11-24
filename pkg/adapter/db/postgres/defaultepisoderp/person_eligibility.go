package defaultepisoderp

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.PersonCreditEpisodeReader = txQueryer{}
var _ repo.PersonCreditEpisodeReader = connQueryer{}

func (q txQueryer) HasActiveForCreditOwner(ctx context.Context, customer model.CustomerID) (bool, error) {
	return hasActiveForOwner(ctx, q.Tx, customer)
}
func (q connQueryer) HasActiveForCreditOwner(ctx context.Context, customer model.CustomerID) (bool, error) {
	return hasActiveForOwner(ctx, q.Conn, customer)
}
func hasActiveForOwner[Q postgres.Queryer](ctx context.Context, q Q, customer model.CustomerID) (bool, error) {
	const sql = `SELECT EXISTS(SELECT 1 FROM default_episodes
      WHERE credit_owner_id=(SELECT credit_owner_id FROM customer_profiles WHERE id=$1)
        AND status='active') AS active`
	result, err := postgres.QueryOne[struct {
		Active bool `db:"active"`
	}](ctx, q, "legal person has active default", sql, customer)
	return result.Active, err
}
