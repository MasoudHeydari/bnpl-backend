package merchantterminalrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// Retire handles POST /api/v1/merchant/terminal/credentials/{credential_id}/retire.
//
//	@Summary		Retire superseded partner API key for the authenticated merchant
//	@Description	Ends the grace period of a retiring credential of the authenticated merchant's terminal at once.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (merchant_profiles.merchant_id).
//	@Description	Requires bnpl:merchant-terminals:manage:merchant together with the standard gateway tenant and context headers.
//	@Description	Only a retiring credential can be retired; retiring an already retired credential succeeds without change.
//	@Description	The active credential cannot be retired; replace it through rotation. A credential of another terminal is reported as not found.
//	@Tags			Merchant Terminals
//	@Produce		json
//	@Param			credential_id		path		string				true	"Credential identifier"																					format(uuid)
//	@Param			x-auth-subject-id	header		string				true	"Stable authenticated subject identifier issued by Identity Service (merchant_profiles.merchant_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"																				format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"																	format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"																	example(bnpl:merchant-terminals:manage:merchant)
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Success		200					{object}	RetireSuccessSchema	"Credential retired"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		404					{object}	serdser.ErrorResponse
//	@Failure		409					{object}	serdser.ErrorResponse
//	@Failure		422					{object}	serdser.ErrorResponse
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
//	@Header			409					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			409					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			409					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/merchant/terminal/credentials/{credential_id}/retire [post]
func (rs *resource) Retire(c *gin.Context) {
	input, ok := toRetireInput(c)
	if !ok {
		return
	}

	output, err := rs.merchantTerminal.Retire(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)

		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeMerchantTerminalCredentialRetired,
		serdser.SuccessMessageMerchantTerminalCredentialRetired,
		toRetireResponse(output),
	)
}
