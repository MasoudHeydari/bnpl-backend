package customerrs

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/pagination"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	corepagination "github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customeraccountuc"
)

// List handles GET /api/v1/contracts.
//
//	@Summary		List current contracts
//	@Description	Returns active or canceling BNPL contracts (purchased orders) owned by the authenticated customer.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (customer_profiles.external_customer_id). Contracts with status=active or canceling are returned.
//	@Description	Newest contracts first (created_at DESC, id DESC). Empty list is 200, not 404.
//	@Description	requested_amount_minor is the persisted total order/purchase price.
//	@Description	Keyset pagination via limit, after_id, and after_created_at.
//	@Tags			Contracts
//	@Produce		json
//	@Param			x-auth-subject-id	header		string								true	"Stable authenticated subject identifier issued by Identity Service (customer_profiles.external_customer_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string								true	"Authenticated tenant UUID"																						format(uuid)
//	@Param			x-auth-context-id	header		string								true	"Authenticated business-context UUID"																			format(uuid)
//	@Param			x-auth-permissions	header		string								true	"Comma-separated gateway permissions"																			example(bnpl:contracts:read:self)
//	@Param			X-Trace-Id			header		string								false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string								false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string								false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			limit				query		int									false	"Page size (default 20, max 100)"						minimum(1)	maximum(100)
//	@Param			after_id			query		string								false	"Keyset cursor: last contract id from previous page"	format(uuid)
//	@Param			after_created_at	query		string								false	"Keyset cursor: last contract created_at (RFC3339Nano) from previous page"
//	@Param			order				query		string								false	"Sort order; always applied as desc for this endpoint"	Enums(asc,desc)
//	@Success		200					{object}	ActiveContractsListSuccessSchema	"Current contracts listed"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		500					{object}	serdser.ErrorResponse
//	@Header			200					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/contracts [get]
func (rs *resource) ListActiveContracts(c *gin.Context) {
	input, ok := toListActiveContractsInput(c)
	if !ok {
		return
	}

	output, err := rs.customerAccount.ListActiveContracts(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.SerKeyset(
		c,
		http.StatusOK,
		serdser.SuccessCodeActiveContractsListed,
		serdser.SuccessMessageActiveContractsListed,
		toActiveContractsKeysetPage(output),
	)
}

func toListActiveContractsInput(c *gin.Context) (customeraccountuc.ListActiveContractsInput, bool) {
	const op = "customerrs.toListActiveContractsInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return customeraccountuc.ListActiveContractsInput{}, false
	}

	page, ok := pagination.ParseKeyset(c)
	if !ok {
		return customeraccountuc.ListActiveContractsInput{}, false
	}

	page.Order = corepagination.OrderDesc

	return customeraccountuc.ListActiveContractsInput{
		ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
		Page:              page,
	}, true
}

type contractResponse struct {
	ID                      uuid.UUID                      `json:"id" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6" format:"uuid"`
	MerchantProfileID       model.MerchantProfileID        `json:"merchant_profile_id" example:"8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c" format:"uuid"`
	MerchantBranchProfileID *model.MerchantBranchProfileID `json:"merchant_branch_profile_id,omitempty" format:"uuid"`
	RequestedAmountMinor    uint64                         `json:"requested_amount_minor" example:"150000000"`
	Currency                model.Currency                 `json:"currency" example:"IRR"`
	InstallmentCount        int                            `json:"installment_count" example:"4"`
	Status                  model.ContractStatus           `json:"status" example:"active" enums:"active,canceling"`
	CreatedAt               time.Time                      `json:"created_at" example:"2026-08-09T12:00:00Z"`
}

func toActiveContractsKeysetPage(output customeraccountuc.ListActiveContractsOutput) corepagination.KeysetPage[contractResponse] {
	items := make([]contractResponse, 0, len(output.Contracts))
	for _, item := range output.Contracts {
		items = append(items, contractResponse{
			ID:                      item.ID.UUID(),
			MerchantProfileID:       item.MerchantProfileID,
			MerchantBranchProfileID: item.MerchantBranchProfileID,
			RequestedAmountMinor:    item.RequestedAmountMinor,
			Currency:                item.Currency,
			InstallmentCount:        item.InstallmentCount,
			Status:                  item.Status,
			CreatedAt:               item.CreatedAt,
		})
	}

	return corepagination.KeysetPage[contractResponse]{
		Data: items,
		Meta: output.Page,
	}
}
