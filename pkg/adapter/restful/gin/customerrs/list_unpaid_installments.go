package customerrs

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/pagination"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	corepagination "github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customeraccountuc"
)

// List handles GET /api/v1/installments.
//
//	@Summary		List unpaid installments
//	@Description	Returns unpaid BNPL installments owned by the authenticated customer across contracts.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (customer_profiles.external_customer_id). Do not send customer identity in path or query.
//	@Description	Only installments with status scheduled, due, or overdue and an attached contract_id are returned.
//	@Description	Nearest due date first (due_at ASC, id ASC). Empty list is 200, not 404.
//	@Description	data.repayment is the customer-wide repayment position and is never scoped to the current page.
//	@Description	data.repayment.next_payable is the server-selected single installment the customer may pay next.
//	@Description	total_amount_minor is an integer minor-unit monetary value in the response currency.
//	@Description	Keyset pagination via limit, after_id, and after_due_at.
//	@Tags			Installments
//	@Produce		json
//	@Param			x-auth-subject-id	header		string								true	"Stable authenticated subject identifier issued by Identity Service (customer_profiles.external_customer_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string								true	"Authenticated tenant UUID"																						format(uuid)
//	@Param			x-auth-context-id	header		string								true	"Authenticated business-context UUID"																			format(uuid)
//	@Param			x-auth-permissions	header		string								true	"Comma-separated gateway permissions"																			example(bnpl:installments:read:self)
//	@Param			X-Trace-Id			header		string								false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string								false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string								false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			limit				query		int									false	"Page size (default 20, max 100)"						minimum(1)	maximum(100)
//	@Param			after_id			query		string								false	"Keyset cursor: last installment id from previous page"	format(uuid)
//	@Param			after_due_at		query		string								false	"Keyset cursor: last installment due_at (RFC3339Nano) from previous page"
//	@Param			order				query		string								false	"Sort order; always applied as asc for this endpoint"	Enums(asc,desc)
//	@Success		200					{object}	UnpaidInstallmentsListSuccessSchema	"Unpaid installments listed"
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
//	@Security		PermissionAuth
//	@Router			/api/v1/installments [get]
func (rs *resource) ListUnpaidInstallments(c *gin.Context) {
	input, ok := toListUnpaidInstallmentsInput(c)
	if !ok {
		return
	}

	output, err := rs.customerAccount.ListUnpaidInstallments(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	serdser.SerKeysetData(
		c,
		http.StatusOK,
		serdser.SuccessCodeUnpaidInstallmentsListed,
		serdser.SuccessMessageUnpaidInstallmentsListed,
		toUnpaidInstallmentsListData(output),
		output.Page,
		len(output.Installments),
	)
}

func toListUnpaidInstallmentsInput(c *gin.Context) (customeraccountuc.ListUnpaidInstallmentsInput, bool) {
	const op = "customerrs.toListUnpaidInstallmentsInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return customeraccountuc.ListUnpaidInstallmentsInput{}, false
	}

	page, ok := pagination.ParseKeysetByDueAt(c)
	if !ok {
		return customeraccountuc.ListUnpaidInstallmentsInput{}, false
	}

	page.Order = corepagination.OrderAsc

	return customeraccountuc.ListUnpaidInstallmentsInput{
		ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
		Page:               page,
	}, true
}

type unpaidInstallmentResponse struct {
	ID                      uuid.UUID                      `json:"id" example:"0a1b2c3d-4e5f-6789-abcd-ef0123456789" format:"uuid"`
	ContractID              model.ContractID               `json:"contract_id" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6" format:"uuid"`
	MerchantProfileID       model.MerchantProfileID        `json:"merchant_profile_id" example:"8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c" format:"uuid"`
	MerchantBranchProfileID *model.MerchantBranchProfileID `json:"merchant_branch_profile_id,omitempty" format:"uuid"`
	InstallmentOrder        int                            `json:"installment_order" example:"2"`
	TotalAmountMinor        uint64                         `json:"total_amount_minor" example:"25000000"`
	Currency                model.Currency                 `json:"currency" example:"IRR"`
	DueAt                   time.Time                      `json:"due_at" example:"2026-09-09T12:00:00Z"`
	Status                  model.InstallmentStatus        `json:"status" example:"due" enums:"scheduled,due,overdue"`
	PaidAt                  *time.Time                     `json:"paid_at,omitempty"`
}

type repaymentDueInstallmentResponse struct {
	ID                uuid.UUID               `json:"id" format:"uuid"`
	ContractID        uuid.UUID               `json:"contract_id" format:"uuid"`
	InstallmentOrder  int                     `json:"installment_order"`
	TotalAmountMinor  uint64                  `json:"total_amount_minor"`
	Currency          model.Currency          `json:"currency"`
	DueAt             time.Time               `json:"due_at"`
	Status            model.InstallmentStatus `json:"status"`
	HasPendingPayment bool                    `json:"has_pending_payment"`
}

type repaymentNextPayableResponse struct {
	OutstandingAmountMinor uint64                  `json:"outstanding_amount_minor"`
	PaymentBlockReason     string                  `json:"payment_block_reason,omitempty"`
	InstallmentID          uuid.UUID               `json:"installment_id" format:"uuid"`
	ContractID             uuid.UUID               `json:"contract_id" format:"uuid"`
	InstallmentOrder       int                     `json:"installment_order"`
	PayableAmountMinor     uint64                  `json:"payable_amount_minor"`
	Currency               model.Currency          `json:"currency"`
	DueAt                  time.Time               `json:"due_at"`
	Status                 model.InstallmentStatus `json:"status"`
	HasPendingPayment      bool                    `json:"has_pending_payment"`
	PayableAmountKnown     bool                    `json:"payable_amount_known"`
}

type repaymentSummaryResponse struct {
	DueInstallments   []repaymentDueInstallmentResponse `json:"due_installments"`
	DueCount          int                               `json:"due_count"`
	UnpaidCount       int                               `json:"unpaid_count"`
	NextPayable       *repaymentNextPayableResponse     `json:"next_payable"`
	HasPendingPayment bool                              `json:"has_pending_payment"`
}

type unpaidInstallmentsListData struct {
	Installments []unpaidInstallmentResponse `json:"installments"`
	Repayment    repaymentSummaryResponse    `json:"repayment"`
}

func toUnpaidInstallmentsListData(
	output customeraccountuc.ListUnpaidInstallmentsOutput,
) unpaidInstallmentsListData {
	items := make([]unpaidInstallmentResponse, 0, len(output.Installments))
	for _, item := range output.Installments {
		items = append(items, unpaidInstallmentResponse{
			ID:                      item.ID.UUID(),
			ContractID:              item.ContractID,
			MerchantProfileID:       item.MerchantProfileID,
			MerchantBranchProfileID: item.MerchantBranchProfileID,
			InstallmentOrder:        item.InstallmentOrder,
			TotalAmountMinor:        item.TotalAmountMinor,
			Currency:                item.Currency,
			DueAt:                   item.DueAt,
			Status:                  item.Status,
			PaidAt:                  item.PaidAt,
		})
	}

	dueItems := make([]repaymentDueInstallmentResponse, 0, len(output.Repayment.DueInstallments))
	for _, item := range output.Repayment.DueInstallments {
		dueItems = append(dueItems, repaymentDueInstallmentResponse{
			ID:                item.ID.UUID(),
			ContractID:        item.ContractID.UUID(),
			InstallmentOrder:  item.InstallmentOrder,
			TotalAmountMinor:  item.TotalAmountMinor,
			Currency:          item.Currency,
			DueAt:             item.DueAt.UTC(),
			Status:            item.Status,
			HasPendingPayment: item.HasPendingPayment,
		})
	}

	var nextPayable *repaymentNextPayableResponse
	if output.Repayment.NextPayable != nil {
		np := output.Repayment.NextPayable
		nextPayable = &repaymentNextPayableResponse{
			InstallmentID:          np.InstallmentID.UUID(),
			ContractID:             np.ContractID.UUID(),
			InstallmentOrder:       np.InstallmentOrder,
			PayableAmountMinor:     np.PayableAmountMinor,
			Currency:               np.Currency,
			DueAt:                  np.DueAt.UTC(),
			Status:                 np.Status,
			HasPendingPayment:      np.HasPendingPayment,
			PayableAmountKnown:     np.PayableAmountKnown,
			OutstandingAmountMinor: np.OutstandingAmountMinor,
			PaymentBlockReason:     np.PaymentBlockReason,
		}
	}

	return unpaidInstallmentsListData{
		Installments: items,
		Repayment: repaymentSummaryResponse{
			DueInstallments:   dueItems,
			DueCount:          output.Repayment.DueCount,
			UnpaidCount:       output.Repayment.UnpaidCount,
			NextPayable:       nextPayable,
			HasPendingPayment: output.Repayment.HasPendingPayment,
		},
	}
}
