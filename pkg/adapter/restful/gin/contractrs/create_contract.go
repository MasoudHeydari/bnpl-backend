package contractrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

// CreateContract handles POST /api/v1/checkout-decisions/:decision_id/contracts.
//
//	@Summary		Create checkout contract
//	@Description	Creates a checkout contract according to the policy snapshot stored on the Decision. When customer action is required, returns redirect_url and expires_at; merchant-funding policies return payment_required=false.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (customer_profiles.identity_subject_id). Do not send customer_id or payment provider selection in the body.
//	@Description	Installment schedule details are not returned from this endpoint; use contract installment read APIs when needed.
//	@Description	For initial-payment policies activation occurs only after an authoritative payment result. Merchant-funding policies create no PaymentAttempt and activate only after authoritative Financial Core merchant funding.
//	@Description	Merchant funding records MerchantEntitlement synchronously before the contract becomes active. On recognition failure BNPL does not retry in the background; the caller may retry the same contract request.
//	@Description	Idempotent on decision: returns 201 on create and 200 when a contract already exists.
//	@Description	Public response never includes Financial Core payment identifiers.
//	@Tags			Contracts
//	@Accept			json
//	@Produce		json
//	@Param			x-auth-subject-id	header		string						true	"Stable authenticated subject identifier issued by Identity Service"	format(uuid)
//	@Param			x-auth-tenant-id	header		string						true	"Authenticated tenant UUID"												format(uuid)
//	@Param			x-auth-context-id	header		string						true	"Authenticated business-context UUID"									format(uuid)
//	@Param			x-auth-permissions	header		string						true	"Comma-separated gateway permissions"									example(bnpl:contracts:create:self)
//	@Param			X-Trace-Id			header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			decision_id			path		string						true	"Checkout decision ID"	Format(uuid)
//	@Param			body				body		CreateContractRequestSchema	false	"Empty body; payment provider selection is not accepted"
//	@Success		201					{object}	CreateContractSuccessSchema	"Contract created"
//	@Success		200					{object}	CreateContractSuccessSchema	"Contract already exists for the decision"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		409					{object}	serdser.ErrorResponse
//	@Failure		422					{object}	serdser.ErrorResponse
//	@Failure		500					{object}	serdser.ErrorResponse
//	@Failure		503					{object}	serdser.ErrorResponse
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
//	@Router			/api/v1/checkout-decisions/{decision_id}/contracts [post]
func (rs *resource) CreateContract(c *gin.Context) {
	input, ok := toCreateContractInput(c)
	if !ok {
		return
	}

	output, err := rs.contract.CreateContract(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	status := http.StatusCreated
	message := serdser.SuccessMessageContractCreated

	if output.Result == contractuc.CreateContractResultAlreadyExists {
		status = http.StatusOK
		message = serdser.SuccessMessageContractAlreadyExists
	}

	serdser.Ser(
		c,
		status,
		serdser.SuccessCodeContractCreated,
		message,
		toCreateContractResponse(output),
	)
}
