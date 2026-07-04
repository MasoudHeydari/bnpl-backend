package customeraccountuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// GetContractInstallments returns the authenticated customer's contract summary and installment list.
//
// Contracts that do not exist or belong to another customer both return
// not-found (missing customer is mapped to ContractNotFound).
//
// NextPayable identifies the earliest unpaid schedule installment by
// installment_order. Later installments are not payable until earlier ones are paid.
func (uc *UseCase) GetContractInstallments(
	ctx context.Context,
	input GetContractInstallmentsInput,
) (GetContractInstallmentsOutput, error) {
	const op = "customeraccountuc.UseCase.GetContractInstallments"

	if input.ContractID.IsZero() {
		return GetContractInstallmentsOutput{}, contractIDRequiredError()
	}

	if input.ExternalCustomerID.IsZero() {
		return GetContractInstallmentsOutput{}, externalCustomerIDRequiredError()
	}

	var output GetContractInstallmentsOutput

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			profileQ := uc.customerProfile.Conn(conn)
			contractQ := uc.contract.Conn(conn)
			installmentQ := uc.installment.Conn(conn)
			installmentChargeQ := uc.installmentCharge.Conn(conn)

			profile, err := profileQ.GetByExternalCustomerID(ctx, input.ExternalCustomerID)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					return contractNotFoundError(err)
				}

				return err
			}

			contract, err := contractQ.GetByIDAndCustomerID(
				ctx,
				input.ContractID,
				profile.ID,
			)
			if err != nil {
				return err
			}

			installments, err := installmentQ.ListByContractID(ctx, contract.ID)
			if err != nil {
				return err
			}

			if contract.InstallmentCount > 0 && len(installments) == 0 {
				return cerr.InternalError(fmt.Errorf(
					"%s: contract %s expects %d installments but none were found", op, contract.ID,
					contract.InstallmentCount,
				))
			}

			views := make([]InstallmentView, 0, len(installments))
			for _, installment := range installments {
				views = append(views, InstallmentView{
					ID:               installment.ID,
					InstallmentOrder: installment.InstallmentOrder,
					TotalAmountMinor: installment.TotalAmountMinor,
					Currency:         installment.Currency,
					DueAt:            installment.DueAt,
					Status:           installment.Status,
					PaidAt:           installment.PaidAt,
				})
			}

			var nextPayable *NextPayableInstallmentView
			if contract.Status.IsActive() {
				var nextErr error
				nextPayable, nextErr = buildNextPayable(ctx, installmentChargeQ, installments)
				if nextErr != nil {
					return nextErr
				}
			}

			output = GetContractInstallmentsOutput{
				ContractID:           contract.ID,
				RequestedAmountMinor: contract.RequestedAmountMinor,
				Currency:             contract.Currency,
				Status:               contract.Status,
				CreatedAt:            contract.CreatedAt,
				NextPayable:          nextPayable,
				Installments:         views,
			}

			return nil
		},
	)
	if err != nil {
		return GetContractInstallmentsOutput{}, err
	}

	return output, nil
}

func buildNextPayable(
	ctx context.Context,
	installmentChargeQ repo.InstallmentChargeConnQueryer,
	installments []model.Installment,
) (*NextPayableInstallmentView, error) {
	next, ok := model.NextPayableInSequence(installments)
	if !ok {
		return nil, nil
	}

	charges, err := installmentChargeQ.ListByInstallmentIDs(ctx, []model.InstallmentID{next.ID})
	if err != nil {
		return nil, err
	}

	payableAmount, payableOK, payableErr := model.CustomerPayableAmount(next, charges)
	if payableErr != nil {
		return nil, payableErr
	}

	if !payableOK {
		payableAmount = 0
	}

	return &NextPayableInstallmentView{
		ID:                 next.ID,
		InstallmentOrder:   next.InstallmentOrder,
		PayableAmountMinor: payableAmount,
		Currency:           next.Currency,
		DueAt:              next.DueAt,
		Status:             next.Status,
	}, nil
}
