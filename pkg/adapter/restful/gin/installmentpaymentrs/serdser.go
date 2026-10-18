package installmentpaymentrs

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
)

type createPaymentAttemptRequest struct {
	Channel string `json:"channel" binding:"required" example:"gateway" enums:"gateway,bank_transfer,wallet"`
}

func toCreatePaymentAttemptInput(c *gin.Context) (installmentrepaymentuc.CreatePaymentAttemptInput, bool) {
	const op = "installmentpaymentrs.toCreatePaymentAttemptInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return installmentrepaymentuc.CreatePaymentAttemptInput{}, false
	}

	idempotencyKey := c.GetHeader(header.IdempotencyKey)
	if idempotencyKey == "" {
		serdser.SerValidationErr(c, header.IdempotencyKey, "validation.idempotency_key_header_required")
		return installmentrepaymentuc.CreatePaymentAttemptInput{}, false
	}

	contractID, err := model.ParseContractID(c.Param("contract_id"))
	if err != nil || contractID.IsZero() {
		serdser.SerValidationErr(c, "contract_id", "validation.contract_id_invalid")
		return installmentrepaymentuc.CreatePaymentAttemptInput{}, false
	}

	installmentID, err := model.ParseInstallmentID(c.Param("installment_id"))
	if err != nil || installmentID.IsZero() {
		serdser.SerValidationErr(c, "installment_id", "validation.installment_id_invalid")
		return installmentrepaymentuc.CreatePaymentAttemptInput{}, false
	}

	var req createPaymentAttemptRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return installmentrepaymentuc.CreatePaymentAttemptInput{}, false
	}

	channel := model.PaymentChannel(req.Channel)
	if !channel.IsValid() {
		serdser.SerValidationErr(c, "channel", "validation.channel_invalid")
		return installmentrepaymentuc.CreatePaymentAttemptInput{}, false
	}

	return installmentrepaymentuc.CreatePaymentAttemptInput{
		TenantID:          authCtx.TenantID,
		ContextID:         authCtx.ContextID,
		ContractID:        contractID,
		InstallmentID:     installmentID,
		ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
		IdempotencyKey:    idempotencyKey,
		Channel:           channel,
	}, true
}

// paymentAttemptResponse is the public BNPL-owned payment attempt projection.
// Financial Core identifiers are never serialized on this boundary.
type paymentAttemptResponse struct {
	ID uuid.UUID `json:"id" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6" format:"uuid"`

	InstallmentID model.InstallmentID `json:"installment_id" example:"0a1b2c3d-4e5f-6789-abcd-ef0123456789" format:"uuid"`

	AmountMinor uint64         `json:"amount_minor" example:"25000000"`
	Currency    model.Currency `json:"currency" example:"IRR"`

	State   model.PaymentAttemptState `json:"state" example:"created"`
	Channel model.PaymentChannel      `json:"channel" example:"gateway"`

	RedirectURL *string `json:"redirect_url,omitempty" example:"https://pay.example/session/abc"`
	ExpiresAt   *string `json:"expires_at,omitempty" example:"2026-08-09T12:30:00Z"`
}

type createPaymentAttemptResponse struct {
	Result         installmentrepaymentuc.CreatePaymentAttemptResult `json:"result" example:"queued"`
	PaymentAttempt paymentAttemptResponse                            `json:"payment_attempt"`
}

func toCreatePaymentAttemptResponse(output installmentrepaymentuc.CreatePaymentAttemptOutput) createPaymentAttemptResponse {
	attempt := output.PaymentAttempt

	response := paymentAttemptResponse{
		ID:            attempt.ID.UUID(),
		InstallmentID: attempt.InstallmentID,
		AmountMinor:   attempt.AmountMinor,
		Currency:      attempt.Currency,
		State:         attempt.State,
		Channel:       attempt.Channel,
		RedirectURL:   attempt.PaymentURL,
	}

	if attempt.FinancialPaymentExpiresAt != nil {
		formatted := attempt.FinancialPaymentExpiresAt.UTC().Format(time.RFC3339)
		response.ExpiresAt = &formatted
	}

	return createPaymentAttemptResponse{
		Result:         output.Result,
		PaymentAttempt: response,
	}
}
