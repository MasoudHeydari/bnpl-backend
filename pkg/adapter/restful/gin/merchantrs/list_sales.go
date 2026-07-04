package merchantrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/merchantuc"
)

// ListSales handles GET /api/v1/merchant/sales.
//
//	@Summary		List merchant sales
//	@Description	Returns active BNPL sales (contracts with status=active) for the authenticated merchant.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (merchant_profiles.merchant_id).
//	@Description	Newest sales first (created_at DESC, id DESC). Empty list is 200, not 404.
//	@Description	Supports both keyset and page-number pagination via pagination=keyset|page (default keyset).
//	@Description	Keyset mode uses limit, after_id, and after_created_at.
//	@Description	Page mode uses page and limit and returns total counts in meta.pagination.
//	@Tags			Merchant Sales
//	@Produce		json
//	@Param			x-auth-subject-id	header		string					true	"Stable authenticated subject identifier issued by Identity Service (merchant_profiles.merchant_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string					true	"Authenticated tenant UUID"																				format(uuid)
//	@Param			x-auth-context-id	header		string					true	"Authenticated business-context UUID"																	format(uuid)
//	@Param			x-auth-permissions	header		string					true	"Comma-separated gateway permissions"																	example(bnpl:contracts:read:merchant)
//	@Param			X-Trace-Id			header		string					false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string					false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string					false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			pagination			query		string					false	"Pagination mode"												Enums(keyset,page)	default(keyset)
//	@Param			limit				query		int						false	"Page size (default 20, max 100)"								minimum(1)			maximum(100)
//	@Param			after_id			query		string					false	"Keyset cursor: last sale id from previous page (keyset mode)"	format(uuid)
//	@Param			after_created_at	query		string					false	"Keyset cursor: last sale created_at (RFC3339Nano) from previous page (keyset mode)"
//	@Param			page				query		int						false	"1-based page number (page mode)"						minimum(1)
//	@Param			order				query		string					false	"Sort order; always applied as desc for this endpoint"	Enums(asc,desc)
//	@Success		200					{object}	SalesListSuccessSchema	"Merchant sales listed"
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
//	@Router			/api/v1/merchant/sales [get]
func (rs *resource) ListSales(c *gin.Context) {
	input, ok := toListSalesInput(c)
	if !ok {
		return
	}

	output, err := rs.merchant.ListSales(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	switch input.Mode {
	case merchantuc.PaginationModePage:
		serdser.SerPage(
			c,
			http.StatusOK,
			serdser.SuccessCodeMerchantSalesListed,
			serdser.SuccessMessageMerchantSalesListed,
			toSalesOffsetPage(output),
		)

	default:
		serdser.SerKeyset(
			c,
			http.StatusOK,
			serdser.SuccessCodeMerchantSalesListed,
			serdser.SuccessMessageMerchantSalesListed,
			toSalesKeysetPage(output),
		)
	}
}
