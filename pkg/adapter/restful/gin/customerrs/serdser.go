package customerrs

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customeraccountuc"
)

func toGetContractInstallmentsInput(c *gin.Context) (customeraccountuc.GetContractInstallmentsInput, bool) {
	const op = "customerrs.toGetContractInstallmentsInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(fmt.Errorf("%s: authentication context is missing", op)))
		return customeraccountuc.GetContractInstallmentsInput{}, false
	}

	contractID, err := model.ParseContractID(c.Param("contract_id"))
	if err != nil || contractID.IsZero() {
		serdser.SerValidationErr(c, "contract_id", "validation.contract_id_invalid")
		return customeraccountuc.GetContractInstallmentsInput{}, false
	}

	return customeraccountuc.GetContractInstallmentsInput{
		ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
		ContractID:        contractID,
	}, true
}

type contractInstallmentResponse struct {
	ID               uuid.UUID               `json:"id" example:"0a1b2c3d-4e5f-6789-abcd-ef0123456789" format:"uuid"`
	InstallmentOrder int                     `json:"installment_order" example:"1"`
	TotalAmountMinor uint64                  `json:"total_amount_minor" example:"75000000"`
	Currency         model.Currency          `json:"currency" example:"IRR"`
	DueAt            time.Time               `json:"due_at" example:"2026-08-09T12:00:00Z"`
	Status           model.InstallmentStatus `json:"status" example:"paid"`
	PaidAt           *time.Time              `json:"paid_at,omitempty" example:"2026-08-09T12:05:00Z"`
}

type nextPayableInstallmentResponse struct {
	ID                 uuid.UUID               `json:"id" example:"1b2c3d4e-5f67-8901-bcde-f01234567890" format:"uuid"`
	InstallmentOrder   int                     `json:"installment_order" example:"2"`
	PayableAmountMinor uint64                  `json:"payable_amount_minor" example:"25000000"`
	Currency           model.Currency          `json:"currency" example:"IRR"`
	DueAt              time.Time               `json:"due_at" example:"2026-09-09T12:00:00Z"`
	Status             model.InstallmentStatus `json:"status" example:"due"`
}

type getResponse struct {
	ContractID             model.ContractID                `json:"contract_id" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6" format:"uuid"`
	RequestedAmountMinor   uint64                          `json:"requested_amount_minor" example:"150000000"`
	Currency               model.Currency                  `json:"currency" example:"IRR"`
	Status                 model.ContractStatus            `json:"status" example:"active" enums:"active,canceling"`
	CreatedAt              time.Time                       `json:"created_at" example:"2026-08-09T12:00:00Z"`
	NextPayableInstallment *nextPayableInstallmentResponse `json:"next_payable_installment,omitempty"`
	Installments           []contractInstallmentResponse   `json:"installments"`
}

func toGetResponse(output customeraccountuc.GetContractInstallmentsOutput) getResponse {
	installments := make([]contractInstallmentResponse, 0, len(output.Installments))
	for _, item := range output.Installments {
		installments = append(installments, contractInstallmentResponse{
			ID:               item.ID.UUID(),
			InstallmentOrder: item.InstallmentOrder,
			TotalAmountMinor: item.TotalAmountMinor,
			Currency:         item.Currency,
			DueAt:            item.DueAt,
			Status:           item.Status,
			PaidAt:           item.PaidAt,
		})
	}

	var nextPayable *nextPayableInstallmentResponse
	if output.NextPayable != nil {
		nextPayable = &nextPayableInstallmentResponse{
			ID:                 output.NextPayable.ID.UUID(),
			InstallmentOrder:   output.NextPayable.InstallmentOrder,
			PayableAmountMinor: output.NextPayable.PayableAmountMinor,
			Currency:           output.NextPayable.Currency,
			DueAt:              output.NextPayable.DueAt,
			Status:             output.NextPayable.Status,
		}
	}

	return getResponse{
		ContractID:             output.ContractID,
		RequestedAmountMinor:   output.RequestedAmountMinor,
		Currency:               output.Currency,
		Status:                 output.Status,
		CreatedAt:              output.CreatedAt,
		NextPayableInstallment: nextPayable,
		Installments:           installments,
	}
}
