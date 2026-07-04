package customeraccountuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ListActiveContracts returns the authenticated customer's current contracts (active or canceling), newest first.
//
// An empty list is a successful result, including when no customer profile exists
// for the external subject ID.
func (uc *UseCase) ListActiveContracts(ctx context.Context, input ListActiveContractsInput) (ListActiveContractsOutput, error) {
	if input.ExternalCustomerID.IsZero() {
		return ListActiveContractsOutput{}, externalCustomerIDRequiredError()
	}

	page := input.Page
	page.Order = pagination.OrderDesc
	page.Normalize()
	if err := page.Validate(); err != nil {
		return ListActiveContractsOutput{}, invalidPaginationError(err)
	}

	fetchLimit := page.Limit + 1
	fetchPage := page
	fetchPage.Limit = fetchLimit

	var contracts []ActiveContract

	err := uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			customerProfileQ := uc.customerProfile.Conn(conn)
			profile, err := customerProfileQ.GetByExternalCustomerID(ctx, input.ExternalCustomerID)
			if err != nil {
				if cerr.IsCode(err, cerr.CodeResourceNotFound) {
					contracts = nil
					return nil
				}

				return err
			}

			contractQ := uc.contract.Conn(conn)
			rows, listErr := contractQ.ListActiveByCustomerID(
				ctx,
				profile.ID,
				fetchPage,
			)
			if listErr != nil {
				return listErr
			}

			contracts = make([]ActiveContract, 0, len(rows))
			for _, row := range rows {
				contracts = append(contracts, ActiveContract{
					ID:                      row.ID,
					MerchantProfileID:       row.MerchantProfileID,
					MerchantBranchProfileID: row.MerchantBranchProfileID,
					RequestedAmountMinor:    row.RequestedAmountMinor,
					Currency:                row.Currency,
					InstallmentCount:        row.InstallmentCount,
					Status:                  row.Status,
					CreatedAt:               row.CreatedAt,
				})
			}

			return nil
		},
	)
	if err != nil {
		return ListActiveContractsOutput{}, err
	}

	hasNext := len(contracts) > page.Limit
	if hasNext {
		contracts = contracts[:page.Limit]
	}

	meta := pagination.KeysetMeta{
		PaginationType: "keyset",
		Limit:          page.Limit,
		HasNext:        hasNext,
	}
	if hasNext && len(contracts) > 0 {
		last := contracts[len(contracts)-1]
		afterID := last.ID.String()
		afterCreatedAt := last.CreatedAt.UTC().Format(time.RFC3339Nano)
		meta.NextAfterID = &afterID
		meta.NextAfterCreatedAt = &afterCreatedAt
	}

	return ListActiveContractsOutput{
		Contracts: contracts,
		Page:      meta,
	}, nil
}
