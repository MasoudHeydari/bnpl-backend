package merchantrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// ListCreditKinds handles GET /api/v1/admin/merchants/{merchant_profile_id}/credit-kinds.
//
//	@Summary		List merchant credit kind eligibility (admin)
//	@Description	Returns the effective eligibility of pay_in_4 and pay_next_month for a merchant.
//	@Description	pay_in_4 is enabled unless explicitly disabled (legacy compatibility); pay_next_month is disabled unless explicitly enabled.
//	@Description	Requires bnpl:merchants:read:any. merchant_profile_id is the internal BNPL merchant profile id (merchant_profiles.id).
//	@Tags			Merchant Credit Products Admin
//	@Produce		json
//	@Param			merchant_profile_id	path		string	true	"Internal merchant profile UUID"	format(uuid)
//	@Param			x-auth-subject-id	header		string				true	"Authenticated administrator subject UUID issued by Identity Service"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"	format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"	format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"	example(bnpl:merchants:read:any)
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Success		200					{object}	ListCreditKindsSuccessSchema	"Credit kind eligibility returned"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		404					{object}	serdser.ErrorResponse
//	@Failure		422					{object}	serdser.ErrorResponse
//	@Failure		500					{object}	serdser.ErrorResponse
//	@Header			200					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/admin/merchants/{merchant_profile_id}/credit-kinds [get]
func (rs *resource) ListCreditKinds(c *gin.Context) {
	input, ok := toListCreditKindsInput(c)
	if !ok {
		return
	}

	output, err := rs.creditKind.ListCreditKinds(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)

		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeMerchantCreditKindsListed,
		serdser.SuccessMessageMerchantCreditKindsListed,
		toListCreditKindsResponse(output),
	)
}
