package contractuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func validateCashObligations(ctx context.Context, q repo.InstallmentChargeTxQueryer, installments []model.Installment) (map[model.InstallmentID]uint64, error) {
	charges, err := q.ListByInstallmentIDs(ctx, installmentIDs(installments))
	if err != nil {
		return nil, err
	}

	if err = model.ValidateCashObligations(installments, charges); err != nil {
		return nil, cerr.NewPublic(cerr.CodeConflict, "error.installment_financial_adjustment_required", err, cerr.Details{})
	}
	amounts := map[model.InstallmentID]uint64{}
	byID := map[model.InstallmentID][]model.InstallmentCharge{}
	for _, c := range charges {
		byID[c.InstallmentID] = append(byID[c.InstallmentID], c)
	}
	for _, i := range installments {
		amount, _, _ := model.CustomerPayableAmount(i, byID[i.ID])
		amounts[i.ID] = amount
	}
	return amounts, nil
}
