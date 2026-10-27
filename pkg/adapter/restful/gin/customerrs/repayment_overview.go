package customerrs

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customeraccountuc"
)

type repaymentOverviewInstallmentResponse struct {
	ID                  uuid.UUID               `json:"id" format:"uuid"`
	ContractID          uuid.UUID               `json:"contract_id" format:"uuid"`
	InstallmentOrder    int                     `json:"installment_order"`
	TotalAmountMinor    uint64                  `json:"total_amount_minor"`
	Currency            model.Currency          `json:"currency"`
	DueAt               time.Time               `json:"due_at"`
	Status              model.InstallmentStatus `json:"status"`
	IsPriority          bool                    `json:"is_priority"`
	IsPayableInSequence bool                    `json:"is_payable_in_sequence"`
	HasPendingPayment   bool                    `json:"has_pending_payment"`
}

type priorityRepaymentTargetResponse struct {
	InstallmentID    uuid.UUID               `json:"installment_id" format:"uuid"`
	ContractID       uuid.UUID               `json:"contract_id" format:"uuid"`
	DueAt            time.Time               `json:"due_at"`
	TotalAmountMinor uint64                  `json:"total_amount_minor"`
	Currency         model.Currency          `json:"currency"`
	Status           model.InstallmentStatus `json:"status"`
}

type repaymentOverviewResponse struct {
	Installments []repaymentOverviewInstallmentResponse `json:"installments"`
	Priority     *priorityRepaymentTargetResponse       `json:"priority"`
}

// GetRepaymentOverview handles GET /api/v1/repayment-overview.
//
//	@Summary		Get repayment overview
//	@Description	Returns unpaid installments and the server-selected priority repayment target for the authenticated customer.
//	@Description	x-auth-subject-id is the Identity Service subject ID (customer_profiles.external_customer_id).
//	@Description	Empty unpaid list is 200 with data.installments=[].
//	@Tags			Repayments
//	@Produce		json
//	@Param			x-auth-subject-id	header		string	true	"Stable authenticated subject identifier issued by Identity Service (customer_profiles.external_customer_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string	true	"Authenticated tenant UUID"																						format(uuid)
//	@Param			x-auth-context-id	header		string	true	"Authenticated business-context UUID"																			format(uuid)
//	@Param			x-auth-permissions	header		string	true	"Comma-separated gateway permissions"																			example(bnpl:installments:read:self)
//	@Param			X-Trace-Id			header		string	false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string	false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string	false	"Optional; reused when present otherwise generated. Always returned."
//	@Success		200					{object}	serdser.SuccessResponse
//	@Failure		401					{object}	serdser.ErrorResponse
//	@Failure		403					{object}	serdser.ErrorResponse
//	@Failure		500					{object}	serdser.ErrorResponse
//	@Header			200					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			200					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			401					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			403					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Trace-Id			"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Request-Id		"Always set: reuse inbound value or generate"
//	@Header			500					{string}	X-Correlation-Id	"Always set: reuse inbound value or generate"
//	@Security		PermissionAuth
//	@Router			/api/v1/repayment-overview [get]
func (rs *resource) GetRepaymentOverview(c *gin.Context) {
	const op = "customerrs.GetRepaymentOverview"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return
	}

	output, err := rs.customerAccount.GetRepaymentOverview(
		c.Request.Context(),
		customeraccountuc.GetRepaymentOverviewInput{
			ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
		},
	)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.Ser(
		c,
		http.StatusOK,
		serdser.SuccessCodeRepaymentOverviewRetrieved,
		serdser.SuccessMessageRepaymentOverviewRetrieved,
		toRepaymentOverviewResponse(output),
	)
}

func toRepaymentOverviewResponse(
	output customeraccountuc.GetRepaymentOverviewOutput,
) repaymentOverviewResponse {
	items := make([]repaymentOverviewInstallmentResponse, 0, len(output.Installments))
	for _, item := range output.Installments {
		items = append(items, repaymentOverviewInstallmentResponse{
			ID:                  item.ID.UUID(),
			ContractID:          item.ContractID.UUID(),
			InstallmentOrder:    item.InstallmentOrder,
			TotalAmountMinor:    item.TotalAmountMinor,
			Currency:            item.Currency,
			DueAt:               item.DueAt.UTC(),
			Status:              item.Status,
			IsPriority:          item.IsPriority,
			IsPayableInSequence: item.IsPayableInSequence,
			HasPendingPayment:   item.HasPendingPayment,
		})
	}

	var priority *priorityRepaymentTargetResponse
	if output.Priority != nil {
		priority = &priorityRepaymentTargetResponse{
			InstallmentID:    output.Priority.InstallmentID.UUID(),
			ContractID:       output.Priority.ContractID.UUID(),
			DueAt:            output.Priority.DueAt.UTC(),
			TotalAmountMinor: output.Priority.TotalAmountMinor,
			Currency:         output.Priority.Currency,
			Status:           output.Priority.Status,
		}
	}

	return repaymentOverviewResponse{
		Installments: items,
		Priority:     priority,
	}
}
