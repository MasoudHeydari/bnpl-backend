package checkoutsessionrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// UpsertReturnDestination handles PUT /api/v1/partner/return-destinations/{destination_name}.
//
//	@Summary		Upsert merchant return destination
//	@Description	Registers or updates a symbolic browser-return destination for the authenticated merchant.
//	@Description	destination_name must match ^[a-z0-9]+(-[a-z0-9]+)*$ and be 1..64 characters (lowercase kebab-case).
//	@Description	Checkout create selects destinations by name only; arbitrary return URLs are never accepted on checkout requests.
//	@Description	Production requires https return_url; development/test may use http only for localhost/loopback.
//	@Description	Leading or trailing whitespace in return_url is rejected (not trimmed).
//	@Description	enabled is optional: omit it to preserve the stored value (true on first registration).
//	@Description	Requires the dedicated bnpl:checkout-return-destinations:manage:merchant permission.
//	@Tags			Checkout Sessions
//	@Accept			json
//	@Produce		json
//	@Param			X-Terminal-Id		header		string									true	"Public numeric terminal identifier of the partner"
//	@Param			X-API-Key			header		string									true	"Secret API key of the partner terminal"
//	@Param			X-Trace-Id			header		string									false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string									false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string									false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			destination_name	path		string									true	"Lowercase kebab-case destination name matching ^[a-z0-9]+(-[a-z0-9]+)*$"	minLength(1)	maxLength(64)
//	@Param			body				body		UpsertReturnDestinationRequestSchema	true	"Return destination configuration"
//	@Success		200					{object}	ReturnDestinationSuccessSchema			"Success"
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
//	@Security		TerminalID && TerminalAPIKey
//	@Router			/api/v1/partner/return-destinations/{destination_name} [put]
func (rs *resource) UpsertReturnDestination(c *gin.Context) {
	input, ok := toUpsertReturnDestinationInput(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.UpsertReturnDestination(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeMerchantReturnDestinationUpserted,
		serdser.SuccessMessageMerchantReturnDestinationUpserted,
		toReturnDestinationResponse(output),
	)
}
