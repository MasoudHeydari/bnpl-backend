package customeraccountuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ListUnpaidInstallments returns the authenticated customer's unpaid installments, nearest due first.
//
// Unpaid means status scheduled, due, or overdue on a contract-attached row.
// An empty list is a successful result, including when no customer profile exists
// for the external subject ID.
func (uc *UseCase) ListUnpaidInstallments(
	ctx context.Context,
	input ListUnpaidInstallmentsInput,
) (ListUnpaidInstallmentsOutput, error) {
	if input.ExternalCustomerID.IsZero() {
		return ListUnpaidInstallmentsOutput{}, externalCustomerIDRequiredError()
	}

	page := input.Page
	page.Order = pagination.OrderAsc
	page.Normalize()
	if err := page.Validate(); err != nil {
		return ListUnpaidInstallmentsOutput{}, invalidPaginationError(err)
	}

	fetchLimit := page.Limit + 1
	fetchPage := page
	fetchPage.Limit = fetchLimit

	var installments []UnpaidInstallment

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			customerProfileQ := uc.customerProfile.Conn(conn)
			profile, err := customerProfileQ.GetByExternalCustomerID(ctx, input.ExternalCustomerID)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					installments = nil
					return nil
				}

				return err
			}

			installmentQ := uc.installment.Conn(conn)
			rows, listErr := installmentQ.ListUnpaidByCustomerID(
				ctx,
				profile.ID,
				fetchPage,
			)
			if listErr != nil {
				return listErr
			}

			installments = make([]UnpaidInstallment, 0, len(rows))
			for _, row := range rows {
				item, mapErr := toUnpaidInstallment(row)
				if mapErr != nil {
					return mapErr
				}

				installments = append(installments, item)
			}

			return nil
		},
	)
	if err != nil {
		return ListUnpaidInstallmentsOutput{}, err
	}

	hasNext := len(installments) > page.Limit
	if hasNext {
		installments = installments[:page.Limit]
	}

	meta := pagination.KeysetMeta{
		PaginationType: "keyset",
		Limit:          page.Limit,
		HasNext:        hasNext,
	}
	if hasNext && len(installments) > 0 {
		last := installments[len(installments)-1]
		afterID := last.ID.String()
		afterDueAt := last.DueAt.UTC().Format(time.RFC3339Nano)
		meta.NextAfterID = &afterID
		meta.NextAfterDueAt = &afterDueAt
	}

	return ListUnpaidInstallmentsOutput{
		Installments: installments,
		Page:         meta,
	}, nil
}

func toUnpaidInstallment(row model.Installment) (UnpaidInstallment, error) {
	const op = "customeraccountuc.toUnpaidInstallment"

	if row.ContractID == nil || row.ContractID.IsZero() {
		return UnpaidInstallment{}, cerr.InternalError(fmt.Errorf(
			"%s: unpaid installment %s is missing contract_id", op, row.ID,
		))
	}

	if !row.Status.IsActive() {
		return UnpaidInstallment{}, cerr.InternalError(fmt.Errorf(
			"%s: unexpected unpaid installment status in list query: %s", op, row.Status,
		))
	}

	return UnpaidInstallment{
		ID:                      row.ID,
		ContractID:              *row.ContractID,
		MerchantProfileID:       row.MerchantProfileID,
		MerchantBranchProfileID: row.MerchantBranchProfileID,
		InstallmentOrder:        row.InstallmentOrder,
		TotalAmountMinor:        row.TotalAmountMinor,
		Currency:                row.Currency,
		DueAt:                   row.DueAt,
		Status:                  row.Status,
		PaidAt:                  row.PaidAt,
	}, nil
}
