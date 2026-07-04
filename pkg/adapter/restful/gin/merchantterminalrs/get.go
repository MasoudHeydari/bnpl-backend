package merchantterminalrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// Get handles GET /api/v1/merchant/terminal.
//
//	@Summary		List partner terminals for the authenticated merchant
//	@Description	Returns the authenticated merchant's partner terminals and current credential metadata.
//	@Description	A merchant currently owns at most one terminal. When none has been provisioned yet, the response is HTTP 200 with an empty data array (not 404).
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (merchant_profiles.merchant_id).
//	@Description	Requires bnpl:merchant-terminals:manage:merchant together with the standard gateway tenant and context headers.
//	@Description	Show-once policy: plaintext api_key is never returned by this read API.
//	@Description	The key is returned only by the initial issuance response (Provision/Rotate) and by an idempotent replay of that same issuance request within the configured replay window.
//	@Description	Outside that window a lost key requires Rotate.
//	@Tags			Merchant Terminals
//	@Produce		json
//	@Param			x-auth-subject-id	header		string				true	"Stable authenticated subject identifier issued by Identity Service (merchant_profiles.merchant_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"																				format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"																	format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"																	example(bnpl:merchant-terminals:manage:merchant)
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Success		200					{object}	GetSuccessSchema	"Terminals returned (possibly empty)"
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
//	@Router			/api/v1/merchant/terminal [get]
func (rs *resource) Get(c *gin.Context) {
	input, ok := toGetInput(c)
	if !ok {
		return
	}

	output, err := rs.merchantTerminal.Get(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)

		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeMerchantTerminalRetrieved,
		serdser.SuccessMessageMerchantTerminalReturned,
		toGetResponse(output),
	)
}
