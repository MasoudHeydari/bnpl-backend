package merchantrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// Get handles GET /api/v1/merchant.
//
//	@Summary		Get merchant profile
//	@Description	Returns display attributes and BNPL participation settings for the authenticated merchant.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (merchant_profiles.merchant_id).
//	@Description	Display fields (name, logo, contacts) are stored in BNPL for now and will later move to Merchant Service.
//	@Description	Risk tier is omitted from this merchant-facing response.
//	@Tags			Merchant Profile
//	@Produce		json
//	@Param			x-auth-subject-id	header		string				true	"Stable authenticated subject identifier issued by Identity Service (merchant_profiles.merchant_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"																				format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"																	format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"																	example(bnpl:merchant-profile:read:merchant)
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Success		200					{object}	GetSuccessSchema	"Merchant profile returned"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		404					{object}	serdser.ErrorResponse
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
//	@Header			404					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/merchant [get]
func (rs *resource) Get(c *gin.Context) {
	input, ok := toGetInput(c)
	if !ok {
		return
	}

	output, err := rs.merchant.GetProfile(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeMerchantProfileRetrieved,
		serdser.SuccessMessageMerchantProfileReturned,
		toProfileResponse(output),
	)
}
