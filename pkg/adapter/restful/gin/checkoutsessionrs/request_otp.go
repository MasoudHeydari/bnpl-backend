package checkoutsessionrs

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

type hostedExternalOTPRequest struct {
	EvaluationGeneration int `json:"evaluation_generation" binding:"required,min=1" example:"1"`
}

// hostedExternalOTPResponse is the privacy-safe OTP issuance result.
type hostedExternalOTPResponse struct {
	CheckoutSessionID    string                      `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	Status               model.CheckoutSessionStatus `json:"status" enums:"open,confirming,processing"`
	EvaluationGeneration int                         `json:"evaluation_generation" example:"1"`
	OTPSent              bool                        `json:"otp_sent" example:"true"`
	OTPExpiresAt         *time.Time                  `json:"otp_expires_at,omitempty"`
	OTPNextIssueAt       *time.Time                  `json:"otp_next_issue_at,omitempty" example:"2026-10-20T12:02:00Z"`
	OTPAvailability      string                      `json:"otp_availability,omitempty" example:"awaiting_entry" enums:"awaiting_entry,consumed_wait,locked,request_available,issuance_blocked,not_applicable"`
}

func toHostedExternalOTPResponse(
	output checkoutsessionuc.RequestExternalOTPOutput,
) hostedExternalOTPResponse {
	return hostedExternalOTPResponse{
		CheckoutSessionID:    output.Token.String(),
		Status:               output.Status,
		EvaluationGeneration: output.EvaluationGeneration,
		OTPSent:              output.OTPSent,
		OTPExpiresAt:         output.OTPExpiresAt,
		OTPNextIssueAt:       output.OTPNextIssueAt,
		OTPAvailability:      output.OTPAvailability,
	}
}

// RequestHostedExternalOTP handles POST /api/v1/checkout-sessions/{token}/otp.
//
//	@Summary		Request hosted checkout OTP
//	@Description	Unauthenticated token API. Issues or idempotently returns the current SMS OTP for initial verification or payment continuation. Never prepares offers, never creates financial operations, and never calls Financial Core. Does not send SMS while an unused OTP is valid or while the original issuance window is still active.
//	@Tags			Checkout Sessions
//	@Accept			json
//	@Produce		json
//	@Param			token	path		string							true	"Public checkout session token (cs_<64 hex>)"
//	@Param			body	body		HostedExternalOTPRequestSchema	true	"Offer generation the OTP must authorize"
//	@Success		200		{object}	HostedExternalOTPSuccessSchema	"Success"
//	@Failure		400		{object}	serdser.ErrorResponse
//	@Failure		404		{object}	serdser.ErrorResponse
//	@Failure		409		{object}	serdser.ErrorResponse
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
//	@Header			409		{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			409		{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			409		{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500		{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500		{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500		{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Router			/api/v1/checkout-sessions/{token}/otp [post]
func (rs *resource) RequestHostedExternalOTP(c *gin.Context) {
	token, ok := checkoutTokenParam(c)
	if !ok {
		return
	}

	var req hostedExternalOTPRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return
	}

	output, err := rs.checkoutSession.RequestExternalOTP(
		c.Request.Context(),
		checkoutsessionuc.RequestExternalOTPInput{
			Token:                token,
			EvaluationGeneration: req.EvaluationGeneration,
		},
	)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeCheckoutOTPRequested,
		serdser.SuccessMessageCheckoutOTPRequested,
		toHostedExternalOTPResponse(output),
	)
}
