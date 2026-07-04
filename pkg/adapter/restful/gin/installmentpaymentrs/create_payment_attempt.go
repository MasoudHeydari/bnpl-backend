package installmentpaymentrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
)

// CreatePaymentAttempt handles POST /api/v1/contracts/:contract_id/installments/:installment_id/pay.
//
//	@Summary		Create installment payment attempt
//	@Description	Queues an installment repayment attempt. When browser action is required, the BNPL payment attempt projection includes redirect_url and expires_at.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (customer_profiles.external_customer_id). Do not send customer_id or payment provider selection in the body.
//	@Description	Installments must be paid in sequence: only the earliest unpaid schedule installment for the contract may be paid.
//	@Description	Payment success is determined by authoritative backend processing, never by browser redirect alone. Requires Idempotency-Key.
//	@Description	Returns 202 when newly queued and 200 on idempotent replay.
//	@Description	Public response never includes Financial Core payment identifiers.
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
//	@Param			contract_id			path		string								true	"Contract ID"		Format(uuid)
//	@Param			installment_id		path		string								true	"Installment ID"	Format(uuid)
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
//	@Router			/api/v1/contracts/{contract_id}/installments/{installment_id}/pay [post]
func (rs *resource) CreatePaymentAttempt(c *gin.Context) {
	input, ok := toCreatePaymentAttemptInput(c)
	if !ok {
		return
	}

	output, err := rs.installmentRepayment.CreatePaymentAttempt(c.Request.Context(), input)
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
		toCreatePaymentAttemptResponse(output),
	)
}
