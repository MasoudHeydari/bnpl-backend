package merchantterminalrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// Rotate handles POST /api/v1/merchant/terminal/credentials/rotate.
//
//	@Summary		Rotate partner API key for the authenticated merchant
//	@Description	Issues a new API key for the authenticated merchant's terminal. The previous key stays accepted for the grace period.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (merchant_profiles.merchant_id).
//	@Description	Requires bnpl:merchant-terminals:manage:merchant together with the standard gateway tenant and context headers.
//	@Description	Show-once: the plaintext api_key is returned only by this issuance response and by an idempotent replay of the same Idempotency-Key within the configured replay window.
//	@Description	It is never exposed by read APIs (GET). Store it securely immediately. A lost key requires rotating again.
//	@Description	grace_period_seconds is optional: omit it for the server default; 0 retires the previous key immediately.
//	@Description	Idempotent per Idempotency-Key: a retry within the issuance replay window returns the same new API key with replayed=true.
//	@Description	A rotation is refused with 409 while a previous rotation is still inside its grace period.
//	@Tags			Merchant Terminals
//	@Accept			json
//	@Produce		json
//	@Param			x-auth-subject-id	header		string				true	"Stable authenticated subject identifier issued by Identity Service (merchant_profiles.merchant_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"																				format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"																	format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"																	example(bnpl:merchant-terminals:manage:merchant)
//	@Param			Idempotency-Key		header		string				true	"Idempotency token (max 128 characters); retries with the same key return the same API key"
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			body				body		RotateRequestSchema	false	"Optional rotation settings"
//	@Success		201					{object}	RotateSuccessSchema	"Credential rotated"
//	@Success		200					{object}	RotateSuccessSchema	"Idempotent replay of an earlier rotation"
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
//	@Header			201					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			201					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			201					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
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
//	@Router			/api/v1/merchant/terminal/credentials/rotate [post]
func (rs *resource) Rotate(c *gin.Context) {
	input, ok := toRotateInput(c)
	if !ok {
		return
	}

	output, err := rs.merchantTerminal.Rotate(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)

		return
	}

	status := http.StatusCreated
	if output.Replayed {
		status = http.StatusOK
	}

	c.Header("Cache-Control", cacheControlNoStore)

	serdser.Ser(
		c,
		status,
		serdser.SuccessCodeMerchantTerminalCredentialRotated,
		serdser.SuccessMessageMerchantTerminalCredentialRotated,
		toRotateResponse(output),
	)
}
