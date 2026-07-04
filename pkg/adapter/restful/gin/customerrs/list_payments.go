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

// publicPaymentStatus is the user-facing finalized payment status.
type publicPaymentStatus string

const (
	publicStatusSucceeded publicPaymentStatus = "succeeded"
	publicStatusFailed    publicPaymentStatus = "failed"
	publicStatusCanceled  publicPaymentStatus = "canceled"
)

// publicPaymentPurpose is the user-facing payment purpose.
type publicPaymentPurpose string

const (
	publicPurposeFirstInstallment publicPaymentPurpose = "first_installment"
	publicPurposeInstallment      publicPaymentPurpose = "installment"
)

// List handles GET /api/v1/payments.
//
//	@Summary		List installment payment history
//	@Description	Returns finalized installment payment attempts owned by the authenticated customer.
//	@Description	x-auth-subject-id is the stable Identity Service subject ID (customer_profiles.external_customer_id). Do not send customer identity in path or query.
//	@Description	Only payment attempts in states succeeded, failed, or canceled are returned.
//	@Description	In-progress and operational states (created, submitted, reconciliation_required, manual_review_required) are excluded and never mapped to another public status.
//	@Description	Includes purposes checkout_first_installment (first_installment) and installment_repayment (installment).
//	@Description	Multiple finalized attempts for the same installment remain visible as separate rows.
//	@Description	Newest attempts first (created_at DESC, id DESC). Empty list is 200, not 404.
//	@Description	amount_minor is an integer minor-unit monetary value in the response currency.
//	@Description	completed_at is state-specific when present: succeeded uses applied_at, failed uses failed_at, canceled uses canceled_at; otherwise null.
//	@Description	Raw provider, reconciliation, and gateway failure details are not exposed.
//	@Description	Keyset pagination via limit, after_id, and after_created_at.
//	@Tags			Installment Payments
//	@Produce		json
//	@Param			x-auth-subject-id	header		string						true	"Stable authenticated subject identifier issued by Identity Service (customer_profiles.external_customer_id)"	format(uuid)
//	@Param			x-auth-tenant-id	header		string						true	"Authenticated tenant UUID"																						format(uuid)
//	@Param			x-auth-context-id	header		string						true	"Authenticated business-context UUID"																			format(uuid)
//	@Param			x-auth-permissions	header		string						true	"Comma-separated gateway permissions"																			example(bnpl:payments:read:self)
//	@Param			X-Trace-Id			header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Request-Id		header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			X-Correlation-Id	header		string						false	"Optional; reused when present otherwise generated. Always returned."
//	@Param			limit				query		int							false	"Page size (default 20, max 100)"					minimum(1)	maximum(100)
//	@Param			after_id			query		string						false	"Keyset cursor: last payment id from previous page"	format(uuid)
//	@Param			after_created_at	query		string						false	"Keyset cursor: last payment created_at (RFC3339Nano) from previous page"
//	@Param			order				query		string						false	"Sort order; always applied as desc for this endpoint"	Enums(asc,desc)
//	@Success		200					{object}	PaymentsListSuccessSchema	"Installment payment history listed"
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
//	@Router			/api/v1/payments [get]
func (rs *resource) ListPayments(c *gin.Context) {
	input, ok := toListPaymentsInput(c)
	if !ok {
		return
	}

	output, err := rs.customerAccount.ListPayments(c.Request.Context(), input)
	if err != nil {
		serdser.SerErr(c, err)
		return
	}

	page, mapErr := toPaymentsKeysetPage(output)
	if mapErr != nil {
		serdser.SerErr(c, mapErr)
		return
	}

	serdser.SerKeyset(
		c,
		http.StatusOK,
		serdser.SuccessCodeInstallmentPaymentsListed,
		serdser.SuccessMessageInstallmentPaymentHistoryListed,
		page,
	)
}

func toListPaymentsInput(c *gin.Context) (customeraccountuc.ListPaymentsInput, bool) {
	const op = "customerrs.toListPaymentsInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return customeraccountuc.ListPaymentsInput{}, false
	}

	page, ok := pagination.ParseKeyset(c)
	if !ok {
		return customeraccountuc.ListPaymentsInput{}, false
	}

	page.Order = corepagination.OrderDesc

	return customeraccountuc.ListPaymentsInput{
		ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
		Page:               page,
	}, true
}

type paymentResponse struct {
	ID            uuid.UUID            `json:"id" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6" format:"uuid"`
	ContractID    model.ContractID     `json:"contract_id" example:"7c9e6679-7425-40de-944b-e07fc1f90ae7" format:"uuid"`
	InstallmentID model.InstallmentID  `json:"installment_id" example:"0a1b2c3d-4e5f-6789-abcd-ef0123456789" format:"uuid"`
	AmountMinor   uint64               `json:"amount_minor" example:"25000000"`
	Currency      model.Currency       `json:"currency" example:"IRR"`
	Status        publicPaymentStatus  `json:"status" example:"succeeded" enums:"succeeded,failed,canceled"`
	Purpose       publicPaymentPurpose `json:"purpose" example:"installment" enums:"first_installment,installment"`
	CreatedAt     time.Time            `json:"created_at" example:"2026-08-09T12:00:00Z"`
	CompletedAt   *time.Time           `json:"completed_at,omitempty" example:"2026-08-09T12:05:00Z"`
}

func toPaymentsKeysetPage(
	output customeraccountuc.ListPaymentsOutput,
) (corepagination.KeysetPage[paymentResponse], error) {
	items := make([]paymentResponse, 0, len(output.Payments))
	for _, item := range output.Payments {
		status, err := toPublicStatus(item.Status)
		if err != nil {
			return corepagination.KeysetPage[paymentResponse]{}, err
		}

		purpose, err := toPublicPurpose(item.Purpose)
		if err != nil {
			return corepagination.KeysetPage[paymentResponse]{}, err
		}

		items = append(items, paymentResponse{
			ID:            item.ID.UUID(),
			ContractID:    item.ContractID,
			InstallmentID: item.InstallmentID,
			AmountMinor:   item.AmountMinor,
			Currency:      item.Currency,
			Status:        status,
			Purpose:       purpose,
			CreatedAt:     item.CreatedAt,
			CompletedAt:   item.CompletedAt,
		})
	}

	return corepagination.KeysetPage[paymentResponse]{
		Data: items,
		Meta: output.Page,
	}, nil
}

func toPublicStatus(state model.PaymentAttemptState) (publicPaymentStatus, error) {
	const op = "customerrs.toPublicStatus"

	switch state {
	case model.SucceededPaymentAttemptState:
		return publicStatusSucceeded, nil

	case model.FailedPaymentAttemptState:
		return publicStatusFailed, nil

	case model.CanceledPaymentAttemptState:
		return publicStatusCanceled, nil

	default:
		return "", cerr.InternalError(fmt.Errorf(
			"%s: unexpected payment attempt state for public history: %s", op, state,
		))
	}
}

func toPublicPurpose(purpose model.FinancialCorePaymentPurpose) (publicPaymentPurpose, error) {
	const op = "customerrs.toPublicPurpose"

	switch purpose {
	case model.CheckoutFirstInstallmentFinancialCorePaymentPurpose:
		return publicPurposeFirstInstallment, nil

	case model.InstallmentRepaymentFinancialCorePaymentPurpose:
		return publicPurposeInstallment, nil

	default:
		return "", cerr.InternalError(fmt.Errorf(
			"%s: unexpected payment attempt purpose for public history: %s", op, purpose,
		))
	}
}
