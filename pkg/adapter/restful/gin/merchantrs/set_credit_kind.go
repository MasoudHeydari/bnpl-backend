package merchantrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// SetCreditKind handles PUT /api/v1/admin/merchants/{merchant_profile_id}/credit-kinds/{credit_kind}.
//
//	@Summary		Enable or disable a merchant credit kind (admin)
//	@Description	Sets whether a merchant may offer pay_in_installments or pay_next_month. pay_next_month stays disabled until explicitly enabled here.
//	@Description	Idempotent: repeating the same desired state returns 200 with changed=false and records no audit event.
//	@Description	Only ACTIVE merchants can be changed (409 otherwise). Existing contracts are not affected.
//	@Description	Requires bnpl:merchants:credit-kinds:update:any. A real change writes audit action merchant_credit_kind.set_enabled.
//	@Tags			Merchant Credit Products Admin
//	@Accept			json
//	@Produce		json
//	@Param			merchant_profile_id	path		string							true	"Internal merchant profile UUID"	format(uuid)
//	@Param			credit_kind			path		string							true	"Credit kind"	Enums(pay_in_installments, pay_next_month)
//	@Param			x-auth-subject-id	header		string				true	"Authenticated administrator subject UUID issued by Identity Service"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"	format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"	format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"	example(bnpl:merchants:credit-kinds:update:any)
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			body				body		SetCreditKindRequestSchema	true	"Desired eligibility"
//	@Success		200					{object}	SetCreditKindSuccessSchema	"Eligibility set (changed=false when already in the requested state)"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		404					{object}	serdser.ErrorResponse
//	@Failure		409					{object}	serdser.ErrorResponse
//	@Failure		422					{object}	serdser.ErrorResponse
//	@Failure		500					{object}	serdser.ErrorResponse
//	@Header			200					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			400					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			409					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			409					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			409					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			422					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/admin/merchants/{merchant_profile_id}/credit-kinds/{credit_kind} [put]
func (rs *resource) SetCreditKind(c *gin.Context) {
	input, ok := toSetCreditKindInput(c)
	if !ok {
		return
	}

	output, err := rs.creditKind.SetCreditKindEnabled(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)

		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeMerchantCreditKindUpdated,
		serdser.SuccessMessageMerchantCreditKindUpdated,
		toSetCreditKindResponse(output),
	)
}
