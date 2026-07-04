package checkoutsessionrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// UpsertWebhookDestination handles PUT /api/v1/partner/webhook-destination.
//
//	@Summary		Upsert merchant webhook endpoint
//	@Description	Registers or updates the single lifecycle webhook endpoint for the authenticated merchant (one webhook per merchant; there are no named destinations).
//	@Description	Checkout requests never supply a webhook URL; lifecycle events resolve the destination from this registry.
//	@Description	The signing secret is write-only and is never echoed in the response. Omit signing_secret to keep the stored secret; send a new value (at least 32 bytes) to rotate it. The first registration requires a secret.
//	@Description	enabled is optional: omit it to preserve the stored value (true on first registration).
//	@Description	Requires the dedicated bnpl:checkout-webhooks:manage:merchant permission; an ordinary checkout-create credential cannot re-point or rotate webhooks.
//	@Tags			Checkout Sessions
//	@Accept			json
//	@Produce		json
//	@Param			X-Terminal-Id		header		string									true	"Public numeric terminal identifier of the partner"
//	@Param			X-API-Key			header		string									true	"Secret API key of the partner terminal"
//	@Param			X-Trace-Id			header		string									false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string									false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string									false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			body				body		UpsertWebhookDestinationRequestSchema	true	"Webhook destination configuration"
//	@Success		200					{object}	WebhookDestinationSuccessSchema			"Success"
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
//	@Router			/api/v1/partner/webhook-destination [put]
func (rs *resource) UpsertWebhookDestination(c *gin.Context) {
	input, ok := toUpsertWebhookDestinationInput(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.UpsertWebhookDestination(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeMerchantWebhookDestinationUpserted,
		serdser.SuccessMessageMerchantWebhookDestinationUpserted,
		toWebhookDestinationResponse(output),
	)
}
