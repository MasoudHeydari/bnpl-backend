package merchantuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// ListBranches returns the authenticated merchant's branches (branch profiles).
//
// All branch statuses are included. An empty list is a successful result,
// including when no merchant profile exists for the external subject ID.
func (uc *UseCase) ListBranches(ctx context.Context, input ListBranchesInput) (ListBranchesOutput, error) {
	if input.MerchantID.IsZero() {
		return ListBranchesOutput{}, merchantIDRequiredError()
	}

	if !input.Mode.IsValid() {
		return ListBranchesOutput{}, invalidPaginationModeError()
	}

	switch input.Mode {
	case PaginationModeKeyset:
		return uc.listBranchesKeyset(ctx, input)

	case PaginationModePage:
		return uc.listBranchesPage(ctx, input)

	default:
		return ListBranchesOutput{}, invalidPaginationModeError()
	}
}

func (uc *UseCase) listBranchesKeyset(ctx context.Context, input ListBranchesInput) (ListBranchesOutput, error) {
	page := input.Keyset
	page.Order = pagination.OrderDesc
	page.Normalize()
	if err := page.Validate(); err != nil {
		return ListBranchesOutput{}, invalidPaginationError(err)
	}

	fetchLimit := page.Limit + 1
	fetchPage := page
	fetchPage.Limit = fetchLimit

	branches, err := uc.loadBranches(ctx, input.MerchantID, func(
		ctx context.Context,
		branchQ repo.MerchantBranchProfileConnQueryer,
		merchantID model.MerchantID,
	) ([]model.MerchantBranchProfile, error) {
		return branchQ.ListByMerchantIDKeyset(ctx, merchantID, fetchPage)
	})
	if err != nil {
		return ListBranchesOutput{}, err
	}

	hasNext := len(branches) > page.Limit
	if hasNext {
		branches = branches[:page.Limit]
	}

	meta := pagination.KeysetMeta{
		PaginationType: "keyset",
		Limit:          page.Limit,
		HasNext:        hasNext,
	}
	if hasNext && len(branches) > 0 {
		last := branches[len(branches)-1]
		afterID := last.ID.String()
		afterCreatedAt := last.CreatedAt.UTC().Format(time.RFC3339Nano)
		meta.NextAfterID = &afterID
		meta.NextAfterCreatedAt = &afterCreatedAt
	}

	return ListBranchesOutput{
		Branches: branches,
		Keyset:   &meta,
	}, nil
}

func (uc *UseCase) listBranchesPage(ctx context.Context, input ListBranchesInput) (ListBranchesOutput, error) {
	page := input.Offset
	page.Order = pagination.OrderDesc
	page.Sort = pagination.DefaultSort
	page.Normalize()

	var (
		branches []Branch
		total    int64
	)

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		merchantProfileQ := uc.merchantProfile.Conn(conn)
		profile, err := merchantProfileQ.GetByMerchantID(ctx, input.MerchantID)
		if err != nil {
			if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				branches = nil
				total = 0
				return nil
			}

			return err
		}

		// Branch rows store the external merchant identity, not merchant_profiles.id.
		merchantID := profile.MerchantID

		branchProfileQ := uc.branchProfile.Conn(conn)
		count, countErr := branchProfileQ.CountByMerchantID(ctx, merchantID)
		if countErr != nil {
			return countErr
		}

		total = count

		rows, listErr := branchProfileQ.ListByMerchantIDOffset(ctx, merchantID, page)
		if listErr != nil {
			return listErr
		}

		branches = mapBranches(rows)
		return nil
	})
	if err != nil {
		return ListBranchesOutput{}, err
	}

	meta := pagination.NewPageMeta(page, total)
	return ListBranchesOutput{
		Branches: branches,
		Page:     &meta,
	}, nil
}

func (uc *UseCase) loadBranches(
	ctx context.Context,
	merchantID model.MerchantID,
	listFn func(
		ctx context.Context,
		branchQ repo.MerchantBranchProfileConnQueryer,
		merchantID model.MerchantID,
	) ([]model.MerchantBranchProfile, error),
) ([]Branch, error) {
	var branches []Branch

	err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		merchantProfileQ := uc.merchantProfile.Conn(conn)
		profile, err := merchantProfileQ.GetByMerchantID(ctx, merchantID)
		if err != nil {
			if cerr.IsCode(err, cerr.CodeResourceNotFound) {
				branches = nil
				return nil
			}

			return err
		}

		branchProfileQ := uc.branchProfile.Conn(conn)
		rows, listErr := listFn(ctx, branchProfileQ, profile.MerchantID)
		if listErr != nil {
			return listErr
		}

		branches = mapBranches(rows)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return branches, nil
}

func mapBranches(rows []model.MerchantBranchProfile) []Branch {
	branches := make([]Branch, 0, len(rows))
	for _, row := range rows {
		branches = append(branches, Branch{
			ID:                        row.ID,
			BranchID:                  row.BranchID,
			Type:                      row.Type,
			Status:                    row.Status,
			FundingModels:             row.FundingModels,
			SettlementModels:          row.SettlementModels,
			Currencies:                row.Currencies,
			RiskTierOverride:          row.RiskTierOverride,
			MaxAmountOverrideMinor:    row.MaxAmountOverrideMinor,
			MaxAmountOverrideCurrency: row.MaxAmountOverrideCurrency,
			MaxTenureMonthsOverride:   row.MaxTenureMonthsOverride,
			Jurisdiction:              row.Jurisdiction,
			CreatedAt:                 row.CreatedAt,
			UpdatedAt:                 row.UpdatedAt,
		})
	}

	return branches
}
