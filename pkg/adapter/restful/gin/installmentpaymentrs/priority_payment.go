package installmentpaymentrs

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
)

// CreatePriorityPaymentAttempt handles POST /api/v1/repayments/priority-pay.
//
//	@Summary		Pay the current priority installment
//	@Description	Server-selects the earliest past-due next-payable installment across the customer's contracts and starts a single-installment repayment.
//	@Description	Does not accept installment_id from the client. Requires Idempotency-Key.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (customer_profiles.external_customer_id).
//	@Description	Returns 202 when newly queued and 200 on idempotent replay.
//	@Tags			Payments
//	@Accept			json
//	@Produce		json
//	@Param			x-auth-subject-id	header		string								true	"Stable authenticated subject identifier issued by Identity Service (customer_profiles.external_customer_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string								true	"Authenticated tenant UUID"																						format(uuid)
//	@Param			x-auth-context-id	header		string								true	"Authenticated business-context UUID"																			format(uuid)
//	@Param			x-auth-permissions	header		string								true	"Comma-separated gateway permissions"																			example(bnpl:payments:start:self)
//	@Param			Idempotency-Key		header		string								true	"Client idempotency token (max 256 characters)"
//	@Param			X-Trace-Id			header		string								false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string								false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string								false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			body				body		CreatePaymentAttemptRequestSchema	true	"Payment attempt request"
//	@Success		202					{object}	CreatePaymentAttemptSuccessSchema	"Payment attempt queued"
//	@Success		200					{object}	CreatePaymentAttemptSuccessSchema	"Idempotent replay of an existing payment attempt"
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
//	@Header			202					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			202					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			202					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
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
//	@Router			/api/v1/repayments/priority-pay [post]
func (rs *resource) CreatePriorityPaymentAttempt(c *gin.Context) {
	input, ok := toCreatePriorityPaymentAttemptInput(c)
	if !ok {
		return
	}

	output, err := rs.installmentRepayment.CreatePriorityPaymentAttempt(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	status := http.StatusAccepted
	message := serdser.SuccessMessageInstallmentPaymentAttemptQueued

	if output.Result == installmentrepaymentuc.CreatePaymentAttemptResultAlreadyExists {
		status = http.StatusOK
		message = serdser.SuccessMessageInstallmentPaymentAttemptAlreadyExists
	}

	serdser.Ser(
		c,
		status,
		serdser.SuccessCodePaymentIntentCreated,
		message,
		toCreatePaymentAttemptResponse(installmentrepaymentuc.CreatePaymentAttemptOutput{
			Result:         output.Result,
			PaymentAttempt: output.PaymentAttempt,
		}),
	)
}

func toCreatePriorityPaymentAttemptInput(
	c *gin.Context,
) (installmentrepaymentuc.CreatePriorityPaymentAttemptInput, bool) {
	const op = "installmentpaymentrs.toCreatePriorityPaymentAttemptInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return installmentrepaymentuc.CreatePriorityPaymentAttemptInput{}, false
	}

	idempotencyKey := c.GetHeader(header.IdempotencyKey)
	if idempotencyKey == "" {
		serdser.SerValidationErr(c, header.IdempotencyKey, "validation.idempotency_key_header_required")
		return installmentrepaymentuc.CreatePriorityPaymentAttemptInput{}, false
	}

	var req createPaymentAttemptRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return installmentrepaymentuc.CreatePriorityPaymentAttemptInput{}, false
	}

	channel := model.PaymentChannel(req.Channel)
	if !channel.IsValid() {
		serdser.SerValidationErr(c, "channel", "validation.channel_invalid")
		return installmentrepaymentuc.CreatePriorityPaymentAttemptInput{}, false
	}

	return installmentrepaymentuc.CreatePriorityPaymentAttemptInput{
		TenantID:           authCtx.TenantID,
		ContextID:          authCtx.ContextID,
		ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
		IdempotencyKey:     idempotencyKey,
		Channel:            channel,
	}, true
}
