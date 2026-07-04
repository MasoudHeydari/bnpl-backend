package merchantrs

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/pagination"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	corepagination "github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantuc"
)

func toGetInput(c *gin.Context) (merchantuc.GetProfileInput, bool) {
	const op = "merchantrs.toGetInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(
			c,
			cerr.NewPublic(
				cerr.CodeUnauthorized,
				"error.authentication_context_missing",
				fmt.Errorf("%s: authentication context is missing", op),
				cerr.Details{
					Fields: []string{
						"x-auth-subject-id",
						"x-auth-tenant-id",
						"x-auth-context-id",
					},
					Violations: []cerr.Violation{
						{
							Code:       "REQUIRED",
							MessageKey: "error.trusted_authentication_headers_are_required",
							Fields: []string{
								"x-auth-subject-id",
								"x-auth-tenant-id",
								"x-auth-context-id",
							},
						},
					},
				},
			),
		)
		return merchantuc.GetProfileInput{}, false
	}

	return merchantuc.GetProfileInput{
		MerchantID: authCtx.SubjectID.AsMerchantID(),
	}, true
}

type profileResponse struct {
	MerchantID         model.MerchantID        `json:"merchant_id" example:"8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c" format:"uuid"`
	DisplayName        string                  `json:"display_name" example:"Sample Merchant"`
	LegalName          string                  `json:"legal_name,omitempty" example:"Sample Merchant LLC"`
	LogoURL            string                  `json:"logo_url,omitempty" example:"https://cdn.example/logo.png"`
	SupportPhone       string                  `json:"support_phone,omitempty" example:"+989121234567"`
	SupportEmail       string                  `json:"support_email,omitempty" example:"support@example.com"`
	Status             model.MerchantStatus    `json:"status" example:"active"`
	FundingModels      []model.FundingModel    `json:"funding_models"`
	SettlementModels   []model.SettlementModel `json:"settlement_models"`
	Currencies         []model.Currency        `json:"currencies"`
	MaxAmountMinor     uint64                  `json:"max_amount_minor" example:"500000000"`
	MaxAmountCurrency  model.Currency          `json:"max_amount_currency" example:"IRR"`
	MaxTenureMonths    uint                    `json:"max_tenure_months" example:"12"`
	RequiresBranchMode bool                    `json:"requires_branch_mode" example:"true"`
	Jurisdiction       model.Jurisdiction      `json:"jurisdiction" example:"IR"`
	CreatedAt          time.Time               `json:"created_at" example:"2026-01-01T00:00:00Z"`
	UpdatedAt          time.Time               `json:"updated_at" example:"2026-08-09T12:00:00Z"`
}

func toProfileResponse(output merchantuc.GetProfileOutput) profileResponse {
	return profileResponse{
		MerchantID:         output.MerchantID,
		DisplayName:        output.DisplayName,
		LegalName:          output.LegalName,
		LogoURL:            output.LogoURL,
		SupportPhone:       output.SupportPhone,
		SupportEmail:       output.SupportEmail,
		Status:             output.Status,
		FundingModels:      output.FundingModels,
		SettlementModels:   output.SettlementModels,
		Currencies:         output.Currencies,
		MaxAmountMinor:     output.MaxAmountMinor,
		MaxAmountCurrency:  output.MaxAmountCurrency,
		MaxTenureMonths:    output.MaxTenureMonths,
		RequiresBranchMode: output.RequiresBranchMode,
		Jurisdiction:       output.Jurisdiction,
		CreatedAt:          output.CreatedAt,
		UpdatedAt:          output.UpdatedAt,
	}
}

func toListBranchesInput(c *gin.Context) (merchantuc.ListBranchesInput, bool) {
	const op = "merchantrs.toListBranchesInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return merchantuc.ListBranchesInput{}, false
	}

	mode, ok := parsePaginationMode(c)
	if !ok {
		return merchantuc.ListBranchesInput{}, false
	}

	input := merchantuc.ListBranchesInput{
		MerchantID: authCtx.SubjectID.AsMerchantID(),
		Mode:       mode,
	}

	switch mode {
	case merchantuc.PaginationModePage:
		page, pageOK := pagination.ParseOffset(c)
		if !pageOK {
			return merchantuc.ListBranchesInput{}, false
		}

		page.Order = corepagination.OrderDesc
		page.Sort = corepagination.DefaultSort
		input.Offset = page

	default:
		page, pageOK := pagination.ParseKeyset(c)
		if !pageOK {
			return merchantuc.ListBranchesInput{}, false
		}

		page.Order = corepagination.OrderDesc
		input.Keyset = page
	}

	return input, true
}

func parsePaginationMode(c *gin.Context) (merchantuc.PaginationMode, bool) {
	raw := strings.TrimSpace(c.Query("pagination"))
	if raw == "" {
		return merchantuc.PaginationModeKeyset, true
	}

	mode := merchantuc.PaginationMode(strings.ToLower(raw))
	if !mode.IsValid() {
		serdser.SerValidationErr(c, "pagination", "validation.pagination_mode_invalid")
		return "", false
	}

	return mode, true
}

type branchResponse struct {
	ID                        uuid.UUID                `json:"id" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6" format:"uuid"`
	BranchID                  model.BranchID           `json:"branch_id" example:"6ba7b811-9dad-11d1-80b4-00c04fd430c8" format:"uuid"`
	Type                      model.MerchantBranchType `json:"type" example:"physical" enums:"physical,online"`
	Status                    model.BranchStatus       `json:"status" example:"active"`
	FundingModels             []model.FundingModel     `json:"funding_models"`
	SettlementModels          []model.SettlementModel  `json:"settlement_models"`
	Currencies                []model.Currency         `json:"currencies"`
	RiskTierOverride          *model.RiskTier          `json:"risk_tier_override,omitempty"`
	MaxAmountOverrideMinor    *uint64                  `json:"max_amount_override_minor,omitempty"`
	MaxAmountOverrideCurrency *model.Currency          `json:"max_amount_override_currency,omitempty"`
	MaxTenureMonthsOverride   *uint                    `json:"max_tenure_months_override,omitempty"`
	Jurisdiction              model.Jurisdiction       `json:"jurisdiction" example:"IR"`
	CreatedAt                 time.Time                `json:"created_at" example:"2026-01-01T00:00:00Z"`
	UpdatedAt                 time.Time                `json:"updated_at" example:"2026-08-09T12:00:00Z"`
}

func toBranchResponses(output merchantuc.ListBranchesOutput) []branchResponse {
	items := make([]branchResponse, 0, len(output.Branches))
	for _, item := range output.Branches {
		items = append(items, branchResponse{
			ID:                        item.ID.UUID(),
			BranchID:                  item.BranchID,
			Type:                      item.Type,
			Status:                    item.Status,
			FundingModels:             item.FundingModels,
			SettlementModels:          item.SettlementModels,
			Currencies:                item.Currencies,
			RiskTierOverride:          item.RiskTierOverride,
			MaxAmountOverrideMinor:    item.MaxAmountOverrideMinor,
			MaxAmountOverrideCurrency: item.MaxAmountOverrideCurrency,
			MaxTenureMonthsOverride:   item.MaxTenureMonthsOverride,
			Jurisdiction:              item.Jurisdiction,
			CreatedAt:                 item.CreatedAt,
			UpdatedAt:                 item.UpdatedAt,
		})
	}

	return items
}

func toBranchesKeysetPage(output merchantuc.ListBranchesOutput) corepagination.KeysetPage[branchResponse] {
	meta := corepagination.KeysetMeta{PaginationType: "keyset"}
	if output.Keyset != nil {
		meta = *output.Keyset
	}

	return corepagination.KeysetPage[branchResponse]{
		Data: toBranchResponses(output),
		Meta: meta,
	}
}

func toBranchesOffsetPage(output merchantuc.ListBranchesOutput) corepagination.OffsetPage[branchResponse] {
	meta := corepagination.PageMeta{PaginationType: "page"}
	if output.Page != nil {
		meta = *output.Page
	}

	return corepagination.OffsetPage[branchResponse]{
		Data: toBranchResponses(output),
		Meta: meta,
	}
}

func toListSalesInput(c *gin.Context) (merchantuc.ListSalesInput, bool) {
	const op = "merchantrs.toListSalesInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return merchantuc.ListSalesInput{}, false
	}

	mode, ok := parsePaginationMode(c)
	if !ok {
		return merchantuc.ListSalesInput{}, false
	}

	input := merchantuc.ListSalesInput{
		MerchantID: authCtx.SubjectID.AsMerchantID(),
		Mode:       mode,
	}

	switch mode {
	case merchantuc.PaginationModePage:
		page, pageOK := pagination.ParseOffset(c)
		if !pageOK {
			return merchantuc.ListSalesInput{}, false
		}

		page.Order = corepagination.OrderDesc
		page.Sort = corepagination.DefaultSort
		input.Offset = page

	default:
		page, pageOK := pagination.ParseKeyset(c)
		if !pageOK {
			return merchantuc.ListSalesInput{}, false
		}

		page.Order = corepagination.OrderDesc
		input.Keyset = page
	}

	return input, true
}

type saleResponse struct {
	ID                            uuid.UUID                      `json:"id" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6" format:"uuid"`
	CustomerID                    model.CustomerID               `json:"customer_id" example:"7c9e6679-7425-40de-944b-e07fc1f90ae7" format:"uuid"`
	MerchantBranchProfileID       *model.MerchantBranchProfileID `json:"merchant_branch_profile_id,omitempty" format:"uuid"`
	RequestedAmountMinor          uint64                         `json:"requested_amount_minor" example:"150000000"`
	RemainingPrincipalAmountMinor uint64                         `json:"remaining_principal_amount_minor" example:"100000000"`
	Currency                      model.Currency                 `json:"currency" example:"IRR"`
	InstallmentCount              int                            `json:"installment_count" example:"4"`
	Status                        model.ContractStatus           `json:"status" example:"active" enums:"active"`
	ActivatedAt                   *time.Time                     `json:"activated_at,omitempty" example:"2026-08-09T12:10:00Z"`
	CreatedAt                     time.Time                      `json:"created_at" example:"2026-08-09T12:00:00Z"`
}

func toSaleResponses(output merchantuc.ListSalesOutput) []saleResponse {
	items := make([]saleResponse, 0, len(output.Sales))
	for _, item := range output.Sales {
		items = append(items, saleResponse{
			ID:                            item.ID.UUID(),
			CustomerID:                    item.CustomerID,
			MerchantBranchProfileID:       item.MerchantBranchProfileID,
			RequestedAmountMinor:          item.RequestedAmountMinor,
			RemainingPrincipalAmountMinor: item.RemainingPrincipalAmountMinor,
			Currency:                      item.Currency,
			InstallmentCount:              item.InstallmentCount,
			Status:                        item.Status,
			ActivatedAt:                   item.ActivatedAt,
			CreatedAt:                     item.CreatedAt,
		})
	}

	return items
}

func toSalesKeysetPage(output merchantuc.ListSalesOutput) corepagination.KeysetPage[saleResponse] {
	meta := corepagination.KeysetMeta{PaginationType: "keyset"}
	if output.Keyset != nil {
		meta = *output.Keyset
	}

	return corepagination.KeysetPage[saleResponse]{
		Data: toSaleResponses(output),
		Meta: meta,
	}
}

func toSalesOffsetPage(output merchantuc.ListSalesOutput) corepagination.OffsetPage[saleResponse] {
	meta := corepagination.PageMeta{PaginationType: "page"}
	if output.Page != nil {
		meta = *output.Page
	}

	return corepagination.OffsetPage[saleResponse]{
		Data: toSaleResponses(output),
		Meta: meta,
	}
}
