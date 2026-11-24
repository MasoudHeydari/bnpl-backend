package contractrp

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.PendingPersonContractQueryer = txQueryer{}

func (q txQueryer) GetPaymentPendingByCreditOwnerID(ctx context.Context, ownerID model.CreditOwnerID) (model.Contract, error) {
	const query = `SELECT` + contractColumns + ` FROM contracts
        WHERE credit_owner_id=$1 AND status='payment_pending' LIMIT 1`
	return postgres.QueryOne[model.Contract](
		ctx,
		q.Tx,
		"get legal person pending contract",
		query,
		ownerID,
	)
}
