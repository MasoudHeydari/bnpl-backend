package contractrs

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

func toCreateContractInput(c *gin.Context) (contractuc.CreateContractInput, bool) {
	const op = "contractrs.toCreateContractInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return contractuc.CreateContractInput{}, false
	}

	decisionID, err := model.ParseDecisionID(c.Param("decision_id"))
	if err != nil || decisionID.IsZero() {
		serdser.SerValidationErr(c, "decision_id", "validation.decision_id_invalid")
		return contractuc.CreateContractInput{}, false
	}

	// Public callers never select a payment provider. Any request body is ignored.

	return contractuc.CreateContractInput{
		DecisionID:         decisionID,
		ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
		TenantID:           authCtx.TenantID,
		ContextID:          authCtx.ContextID,
	}, true
}

type contractResponse struct {
	ID     uuid.UUID            `json:"id" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6" format:"uuid"`
	Status model.ContractStatus `json:"status" example:"payment_pending"`
}

// createContractResponse is the public BNPL payment-facade projection for
// contract creation. Financial Core / PSP identifiers are never exposed.
type createContractResponse struct {
	Result          contractuc.CreateContractResult `json:"result" example:"created"`
	Contract        contractResponse                `json:"contract"`
	PaymentRequired bool                            `json:"payment_required" example:"true"`
	RedirectURL     string                          `json:"redirect_url,omitempty" example:"https://pay.example/session/abc"`
	ExpiresAt       string                          `json:"expires_at,omitempty" example:"2026-08-09T12:30:00Z"`
}

func toCreateContractResponse(output contractuc.CreateContractOutput) createContractResponse {
	response := createContractResponse{
		Result:          output.Result,
		Contract:        contractResponse{ID: output.Contract.ID.UUID(), Status: output.Contract.Status},
		PaymentRequired: output.PaymentRequired,
	}
	if output.PaymentRequired {
		response.RedirectURL = strings.TrimSpace(output.Payment.PaymentLink)
		response.ExpiresAt = strings.TrimSpace(output.Payment.ExpiresAt)
	}

	return response
}

type cancelRequest struct {
	Reason string `json:"reason" example:"customer_requested"`
}

type cancelResponse struct {
	ContractID model.ContractID     `json:"contract_id" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6" format:"uuid"`
	Status     model.ContractStatus `json:"status" example:"canceling"`
}

func toCancelInput(c *gin.Context) (contractuc.CancelInput, bool) {
	const op = "contractrs.toCancelInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return contractuc.CancelInput{}, false
	}

	contractID, err := model.ParseContractID(c.Param("contract_id"))
	if err != nil || contractID.IsZero() {
		serdser.SerValidationErr(c, "contract_id", "validation.contract_id_invalid")
		return contractuc.CancelInput{}, false
	}

	idempotencyKey := strings.TrimSpace(c.GetHeader(header.IdempotencyKey))
	if idempotencyKey == "" {
		serdser.SerValidationErr(c, "Idempotency-Key", "validation.idempotency_key_header_required")
		return contractuc.CancelInput{}, false
	}

	var req cancelRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return contractuc.CancelInput{}, false
	}

	return contractuc.CancelInput{
		TenantID:           authCtx.TenantID,
		ContextID:          authCtx.ContextID,
		ContractID:         contractID,
		ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
		IdempotencyKey:     idempotencyKey,
		Reason:             model.ContractCancellationReason(strings.TrimSpace(req.Reason)),
	}, true
}

func toCancelResponse(output contractuc.CancelOutput) cancelResponse {
	return cancelResponse{
		ContractID: output.Contract.ID,
		Status:     output.Contract.Status,
	}
}
