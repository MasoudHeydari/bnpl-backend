package contractrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

// Cancel handles POST /api/v1/contracts/:contract_id/cancel.
//
//	@Summary		Start contract cancellation
//	@Description	Starts cancellation of an active BNPL contract owned by the authenticated customer (active → canceling).
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (customer_profiles.identity_subject_id). Do not send customer identity in path or body.
//	@Description	Financial Core remains authoritative for canceling → canceled. This endpoint never marks a contract canceled.
//	@Description	Requires Idempotency-Key. A new request returns 202 only after Financial Core synchronously accepts the cancellation for processing; temporary handoff failures return 503 while durable retry remains scheduled. Exact replay returns 200.
//	@Tags			Contracts
//	@Accept			json
//	@Produce		json
//	@Param			x-auth-subject-id	header		string				true	"Stable authenticated subject identifier issued by Identity Service (customer_profiles.identity_subject_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"																						format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"																			format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"																			example(bnpl:contracts:cancel:self)
//	@Param			Idempotency-Key		header		string				true	"Client idempotency token (max 256 characters)"
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			contract_id			path		string				true	"Contract ID"	Format(uuid)
//	@Param			body				body		CancelRequestSchema	true	"Cancellation request"
//	@Success		202					{object}	CancelSuccessSchema	"Cancellation started"
//	@Success		200					{object}	CancelSuccessSchema	"Idempotent replay of an existing cancellation"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		404					{object}	serdser.ErrorResponse
//	@Failure		409					{object}	serdser.ErrorResponse
//	@Failure		422					{object}	serdser.ErrorResponse
//	@Failure		500					{object}	serdser.ErrorResponse
//	@Failure		503					{object}	serdser.ErrorResponse
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
//	@Header			503					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			503					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			503					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/contracts/{contract_id}/cancel [post]
func (rs *resource) Cancel(c *gin.Context) {
	input, ok := toCancelInput(c)
	if !ok {
		return
	}

	output, err := rs.contract.Cancel(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	status := http.StatusAccepted
	message := serdser.SuccessMessageContractCancellationStarted

	if output.Result == contractuc.CancelResultCompleted ||
		output.Result == contractuc.CancelResultAlreadyCanceling ||
		output.Result == contractuc.CancelResultAlreadyCanceled {
		status = http.StatusOK
		message = serdser.SuccessMessageContractCancellationAlreadyExists
	}

	serdser.Ser(
		c,
		status,
		serdser.SuccessCodeContractCancellationStarted,
		message,
		toCancelResponse(output),
	)
}
