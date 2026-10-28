package contractuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// lockActivationCustomer serializes finalization of an already accepted financial
// operation with Collections. Acceptance guards run before the FC request;
// finalization must honor authoritative success, even if eligibility later changes.
func (uc *UseCase) lockActivationCustomer(ctx context.Context, tx repo.Tx, id model.ContractID) error {
	row, err := uc.contract.Tx(tx).GetByID(ctx, id)
	if err != nil {
		return err
	}
	_, err = uc.customerProfile.Tx(tx).LockByID(ctx, row.CustomerID)
	return err
}
