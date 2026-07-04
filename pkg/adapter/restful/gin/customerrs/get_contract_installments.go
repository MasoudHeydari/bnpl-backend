package customerrs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
)

// GetContractInstallments handles GET /api/v1/contracts/:contract_id/installments.
//
//	@Summary		List contract installments
//	@Description	Returns the order summary and ordered installment schedule for one BNPL contract owned by the authenticated customer.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (customer_profiles.identity_subject_id). Missing or foreign-owned contracts return 404.
//	@Description	requested_amount_minor is the persisted total order/purchase price. status is the contract lifecycle status.
//	@Description	next_payable_installment is the earliest unpaid schedule installment by installment_order and the only installment currently eligible for repayment; null when none remain.
//	@Description	Installments must be paid in sequence: a later installment cannot be paid while an earlier unpaid schedule installment exists.
//	@Description	Financial Core remains authoritative for monetary truth.
//	@Tags			Contracts
//	@Produce		json
//	@Param			x-auth-subject-id	header		string				true	"Stable authenticated subject identifier issued by Identity Service (customer_profiles.identity_subject_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string				true	"Authenticated tenant UUID"																						format(uuid)
//	@Param			x-auth-context-id	header		string				true	"Authenticated business-context UUID"																			format(uuid)
//	@Param			x-auth-permissions	header		string				true	"Comma-separated gateway permissions"																			example(bnpl:installments:read:self)
//	@Param			X-Trace-Id			header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string				false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			contract_id			path		string				true	"Contract ID"	Format(uuid)
//	@Success		200					{object}	GetSuccessSchema	"Contract installments listed"
//	@Failure		400					{object}	serdser.ErrorResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		404					{object}	serdser.ErrorResponse
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
//	@Header			404					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			404					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/contracts/{contract_id}/installments [get]
func (rs *resource) GetContractInstallments(c *gin.Context) {
	input, ok := toGetContractInstallmentsInput(c)
	if !ok {
		return
	}

	output, err := rs.customerAccount.GetContractInstallments(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeInstallmentsListed,
		serdser.SuccessMessageContractInstallmentsListed,
		toGetResponse(output),
	)
}
