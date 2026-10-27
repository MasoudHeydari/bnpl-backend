package checkoutsessionrs

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// hostedConfirmationStatusResponse is the privacy-safe hosted confirmation poll.
type hostedConfirmationStatusResponse struct {
	CheckoutSessionID  string     `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	Status             string     `json:"status" example:"processing"`
	ConfirmationStatus string     `json:"confirmation_status" example:"processing" enums:"not_started,processing,payment_action_required,payment_processing,reconfirm_required,retry_available,completed,needs_attention,canceled,expired,unavailable"`
	NextAction         string     `json:"next_action" example:"wait"`
	RetryAfterSeconds  *int       `json:"retry_after_seconds,omitempty" example:"3"`
	OTPNextIssueAt     *time.Time `json:"otp_next_issue_at,omitempty" example:"2026-10-18T12:02:00Z"`
	OTPAvailability    string     `json:"otp_availability,omitempty" example:"consumed_wait" enums:"awaiting_entry,consumed_wait,locked,request_available,issuance_blocked,not_applicable"`
}

func toHostedConfirmationStatusResponse(
	output checkoutsessionuc.GetExternalConfirmationStatusOutput,
) hostedConfirmationStatusResponse {
	return hostedConfirmationStatusResponse{
		CheckoutSessionID:  output.Token.String(),
		Status:             string(output.Status),
		ConfirmationStatus: output.ConfirmationStatus,
		NextAction:         output.NextAction,
		RetryAfterSeconds:  output.RetryAfterSeconds,
		OTPNextIssueAt:     output.OTPNextIssueAt,
		OTPAvailability:    output.OTPAvailability,
	}
}

// GetHostedConfirmationStatus handles GET /api/v1/checkout-sessions/{token}/confirmation-status.
//
//	@Summary		Poll hosted checkout confirmation status
//	@Description	Unauthenticated token API. Read-only projection of durable confirmation state after an inconclusive Confirm. Never starts a Saga, creates a Contract/PaymentAttempt, calls Financial Core, or mutates state. Does not expose payment URLs, Contract IDs, Saga IDs, or Financial Core identifiers.
//	@Tags			Checkout Sessions
//	@Produce		json
//	@Param			token	path		string									true	"Public checkout session token (cs_<64 hex>)"
//	@Success		200		{object}	HostedConfirmationStatusSuccessSchema	"Success"
//	@Failure		400		{object}	serdser.ErrorResponse
//	@Failure		404		{object}	serdser.ErrorResponse
//	@Failure		500		{object}	serdser.ErrorResponse
//	@Header			200		{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			200		{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			200		{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			400		{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			400		{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			400		{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			404		{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			404		{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			404		{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500		{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500		{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500		{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Router			/api/v1/checkout-sessions/{token}/confirmation-status [get]
func (rs *resource) GetHostedConfirmationStatus(c *gin.Context) {
	token, ok := checkoutTokenParam(c)
	if !ok {
		return
	}

	output, err := rs.checkoutSession.GetExternalConfirmationStatus(
		c.Request.Context(),
		checkoutsessionuc.GetExternalConfirmationStatusInput{Token: token},
	)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeCheckoutConfirmationStatusRetrieved,
		serdser.SuccessMessageCheckoutConfirmationStatusRetrieved,
		toHostedConfirmationStatusResponse(output),
	)
}
