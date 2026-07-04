package merchantuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ListSales returns the authenticated merchant's active sales (active contracts).
//
// An empty list is a successful result, including when no merchant profile exists
// for the external subject ID.
func (uc *UseCase) ListSales(ctx context.Context, input ListSalesInput) (ListSalesOutput, error) {
	if input.MerchantID.IsZero() {
		return ListSalesOutput{}, merchantIDRequiredError()
	}

	if !input.Mode.IsValid() {
		return ListSalesOutput{}, invalidPaginationModeError()
	}

	switch input.Mode {
	case PaginationModeKeyset:
		return uc.listSalesKeyset(ctx, input)

	case PaginationModePage:
		return uc.listSalesPage(ctx, input)

	default:
		return ListSalesOutput{}, invalidPaginationModeError()
	}
}

func (uc *UseCase) listSalesKeyset(ctx context.Context, input ListSalesInput) (ListSalesOutput, error) {
	page := input.Keyset
	page.Order = pagination.OrderDesc
	page.Normalize()
	if err := page.Validate(); err != nil {
		return ListSalesOutput{}, invalidPaginationError(err)
	}

	fetchLimit := page.Limit + 1
	fetchPage := page
	fetchPage.Limit = fetchLimit

	sales, err := uc.loadSales(ctx, input.MerchantID, func(
		ctx context.Context,
		contractQ repo.ContractConnQueryer,
		merchantProfileID model.MerchantProfileID,
	) ([]model.Contract, error) {
		return contractQ.ListActiveByMerchantProfileID(ctx, merchantProfileID, fetchPage)
	})
	if err != nil {
		return ListSalesOutput{}, err
	}

	hasNext := len(sales) > page.Limit
	if hasNext {
		sales = sales[:page.Limit]
	}

	meta := pagination.KeysetMeta{
		PaginationType: "keyset",
		Limit:          page.Limit,
		HasNext:        hasNext,
	}
	if hasNext && len(sales) > 0 {
		last := sales[len(sales)-1]
		afterID := last.ID.String()
		afterCreatedAt := last.CreatedAt.UTC().Format(time.RFC3339Nano)
		meta.NextAfterID = &afterID
		meta.NextAfterCreatedAt = &afterCreatedAt
	}

	return ListSalesOutput{
		Sales:  sales,
		Keyset: &meta,
	}, nil
}

func (uc *UseCase) listSalesPage(ctx context.Context, input ListSalesInput) (ListSalesOutput, error) {
	page := input.Offset
	page.Order = pagination.OrderDesc
	page.Sort = pagination.DefaultSort
	page.Normalize()

	var (
		sales []Sale
		total int64
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		merchantProfileQ := uc.merchantProfile.Conn(conn)
		profile, err := merchantProfileQ.GetByMerchantID(ctx, input.MerchantID)
		if err != nil {
			if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				sales = nil
				total = 0
				return nil
			}

			return err
		}

		contractQ := uc.contract.Conn(conn)
		count, countErr := contractQ.CountActiveByMerchantProfileID(ctx, profile.ID)
		if countErr != nil {
			return countErr
		}

		total = count

		rows, listErr := contractQ.ListActiveByMerchantProfileIDOffset(
			ctx,
			profile.ID,
			page,
		)
		if listErr != nil {
			return listErr
		}

		sales = mapSales(rows)
		return nil
	})
	if err != nil {
		return ListSalesOutput{}, err
	}

	meta := pagination.NewPageMeta(page, total)
	return ListSalesOutput{
		Sales: sales,
		Page:  &meta,
	}, nil
}

func (uc *UseCase) loadSales(
	ctx context.Context,
	merchantID model.MerchantID,
	listFn func(
		ctx context.Context,
		contractQ repo.ContractConnQueryer,
		merchantProfileID model.MerchantProfileID,
	) ([]model.Contract, error),
) ([]Sale, error) {
	var sales []Sale

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		merchantProfileQ := uc.merchantProfile.Conn(conn)
		profile, err := merchantProfileQ.GetByMerchantID(ctx, merchantID)
		if err != nil {
			if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				sales = nil
				return nil
			}

			return err
		}

		contractQ := uc.contract.Conn(conn)
		rows, listErr := listFn(ctx, contractQ, profile.ID)
		if listErr != nil {
			return listErr
		}

		sales = mapSales(rows)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return sales, nil
}

func mapSales(rows []model.Contract) []Sale {
	sales := make([]Sale, 0, len(rows))
	for _, row := range rows {
		sales = append(sales, Sale{
			ID:                            row.ID,
			CustomerID:                    row.CustomerID,
			MerchantBranchProfileID:       row.MerchantBranchProfileID,
			RequestedAmountMinor:          row.RequestedAmountMinor,
			RemainingPrincipalAmountMinor: row.RemainingPrincipalAmountMinor,
			Currency:                      row.Currency,
			InstallmentCount:              row.InstallmentCount,
			Status:                        row.Status,
			ActivatedAt:                   row.ActivatedAt,
			CreatedAt:                     row.CreatedAt,
		})
	}

	return sales
}
