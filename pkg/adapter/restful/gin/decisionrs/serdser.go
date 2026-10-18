package decisionrs

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
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

// purchaseContextRequest is the checkout purchase surface on the wire.
//
// Discriminator: type
//   - pos: requires pos_gps_id; online_store_id and merchant_order_id are forbidden
//   - online: requires online_store_id; pos_gps_id is forbidden; merchant_order_id is optional (max 256)
//
// OpenAPI Example Value uses the POS variant (type + pos_gps_id only). Online
// fields intentionally have no example tags so they are not injected into that sample.
type purchaseContextRequest struct {
	Type            model.PurchaseChannel `json:"type" binding:"required" example:"pos" enums:"pos,online"`
	PosGpsID        model.PosGpsID        `json:"pos_gps_id,omitempty" example:"6ba7b811-9dad-11d1-80b4-00c04fd430c8" format:"uuid"`
	OnlineStoreID   model.OnlineStoreID   `json:"online_store_id,omitempty" format:"uuid"`
	MerchantOrderID string                `json:"merchant_order_id,omitempty" maxLength:"256"`
}

// validate rejects malformed purchase contexts at the HTTP boundary.
//
// REST validation is fail-fast only: the use case repeats these invariants
// because callers are not limited to the HTTP adapter.
func (req purchaseContextRequest) validate() error {
	return decisionuc.PurchaseContext{
		Channel:         req.Type,
		PosGpsID:        req.PosGpsID,
		OnlineStoreID:   req.OnlineStoreID,
		MerchantOrderID: req.MerchantOrderID,
	}.Validate()
}

type createRequest struct {
	PurchaseContext      purchaseContextRequest `json:"purchase_context" binding:"required"`
	RequestedAmountMinor uint64                 `json:"requested_amount_minor" binding:"required,min=1" example:"150000000"`
	Currency             string                 `json:"currency" binding:"required" example:"IRR" enums:"IRR,EUR,USD"`
}

func toCreateInput(c *gin.Context) (decisionuc.CreateInput, bool) {
	const op = "decisionrs.toCreateInput"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return decisionuc.CreateInput{}, false
	}

	idempotencyKey := c.GetHeader(header.IdempotencyKey)
	if idempotencyKey == "" {
		serdser.SerValidationErr(c, header.IdempotencyKey, "validation.idempotency_key_header_required")
		return decisionuc.CreateInput{}, false
	}

	var req createRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return decisionuc.CreateInput{}, false
	}

	if err := req.PurchaseContext.validate(); err != nil {
		serdser.SerErr(c, err)
		return decisionuc.CreateInput{}, false
	}

	currency := model.Currency(req.Currency)
	if !currency.IsValid() {
		serdser.SerValidationErr(c, "currency", "validation.currency_invalid")
		return decisionuc.CreateInput{}, false
	}

	return decisionuc.CreateInput{
		CheckoutKey:       idempotencyKey,
		ExternalCustomerID: authCtx.SubjectID.AsExternalCustomerID(),
		PurchaseContext: decisionuc.PurchaseContext{
			Channel:         req.PurchaseContext.Type,
			PosGpsID:        req.PurchaseContext.PosGpsID,
			OnlineStoreID:   req.PurchaseContext.OnlineStoreID,
			MerchantOrderID: req.PurchaseContext.MerchantOrderID,
		},
		RequestedAmountMinor: req.RequestedAmountMinor,
		Currency:             currency,
	}, true
}

type decisionResponse struct {
	ID          uuid.UUID `json:"id" example:"7c9e6679-7425-40de-944b-e07fc1f90ae7" format:"uuid"`
	CheckoutKey string    `json:"checkout_key" example:"checkout-idempotency-key-001"`
	// Outcome is the client-facing checkout decision result.
	// Only "approved" or "denied" are returned; persisted "conditional"
	// outcomes are mapped to "approved" for API readability.
	Outcome     string   `json:"outcome" example:"approved" enums:"approved,denied"`
	ReasonCodes []string `json:"reason_codes"`
}

type installmentOfferInstallmentResponse struct {
	ID                   uuid.UUID               `json:"id" example:"0a1b2c3d-4e5f-6789-abcd-ef0123456789" format:"uuid"`
	InstallmentOrder     int                     `json:"installment_order" example:"1"`
	Status               model.InstallmentStatus `json:"status" example:"offered"`
	DueAt                time.Time               `json:"due_at" example:"2026-08-09T12:00:00Z"`
	PrincipalAmountMinor uint64                  `json:"principal_amount_minor" example:"25000000"`
	UpfrontAmountMinor   uint64                  `json:"upfront_amount_minor" example:"50000000"`
	BaseFeeAmountMinor   uint64                  `json:"base_fee_amount_minor" example:"0"`
	TotalAmountMinor     uint64                  `json:"total_amount_minor" example:"75000000"`
}

type installmentOfferResponse struct {
	InstallmentCount        int                                   `json:"installment_count" example:"4"`
	PayableNowAmountMinor   uint64                                `json:"payable_now_amount_minor" example:"75000000"`
	TotalPayableAmountMinor uint64                                `json:"total_payable_amount_minor" example:"150000000"`
	Currency                model.Currency                        `json:"currency" example:"IRR"`
	ExpiresAt               time.Time                             `json:"expires_at" example:"2026-08-09T12:30:00Z"`
	Installments            []installmentOfferInstallmentResponse `json:"installments"`
}

type purchaseContextResponse struct {
	Type            model.PurchaseChannel `json:"type" example:"pos" enums:"pos,online"`
	PosGpsID        *model.PosGpsID       `json:"pos_gps_id,omitempty" example:"6ba7b811-9dad-11d1-80b4-00c04fd430c8" format:"uuid"`
	OnlineStoreID   *model.OnlineStoreID  `json:"online_store_id,omitempty" format:"uuid"`
	MerchantOrderID string                `json:"merchant_order_id,omitempty"`
}

type checkoutActionResponse struct {
	Type               decisionuc.CheckoutActionType `json:"type" example:"confirm_and_pay" enums:"confirm_and_pay,confirm_purchase"`
	PaymentRequired    bool                          `json:"payment_required" example:"true"`
	PaymentAmountMinor uint64                        `json:"payment_amount_minor" example:"75000000"`
}

type createResponse struct {
	Result          decisionuc.CreateResult `json:"result" example:"created"`
	Decision        decisionResponse        `json:"decision"`
	PurchaseContext purchaseContextResponse `json:"purchase_context"`
	CheckoutAction  *checkoutActionResponse `json:"checkout_action,omitempty"`

	RequestedAmountMinor        uint64         `json:"requested_amount_minor" example:"150000000"`
	ApprovedFinancedAmountMinor uint64         `json:"approved_financed_amount_minor" example:"100000000"`
	UpfrontAmountMinor          uint64         `json:"upfront_amount_minor" example:"50000000"`
	Currency                    model.Currency `json:"currency" example:"IRR"`
	AvailableCreditMinor        uint64         `json:"available_credit_minor" example:"250000000"`
	ExpiresAt                   *time.Time     `json:"expires_at,omitempty" example:"2026-08-09T12:30:00Z"`

	InstallmentOffer *installmentOfferResponse `json:"installment_offer,omitempty"`
}

func toCreateResponse(output decisionuc.CreateOutput) createResponse {
	checkoutKey := output.Decision.IdempotencyKey
	if output.Decision.TargetType != model.CheckoutKeyDecisionTargetType {
		checkoutKey = ""
	}

	reasonCodes := output.Decision.ReasonCodes
	if reasonCodes == nil {
		reasonCodes = []string{}
	}

	resp := createResponse{
		Result:          output.Result,
		PurchaseContext: toPurchaseContextResponse(output.PurchaseContext),
		Decision: decisionResponse{
			ID:          output.Decision.ID.UUID(),
			CheckoutKey: checkoutKey,
			Outcome:     mapCheckoutDecisionOutcomeForResponse(output.Decision.Outcome),
			ReasonCodes: reasonCodes,
		},
		RequestedAmountMinor:        output.RequestedAmountMinor,
		ApprovedFinancedAmountMinor: output.ApprovedFinancedAmountMinor,
		UpfrontAmountMinor:          output.UpfrontAmountMinor,
		Currency:                    output.Currency,
		AvailableCreditMinor:        output.AvailableCreditMinor,
		ExpiresAt:                   output.ExpiresAt,
	}

	if output.CheckoutAction != nil {
		resp.CheckoutAction = &checkoutActionResponse{
			Type:               output.CheckoutAction.Type,
			PaymentRequired:    output.CheckoutAction.PaymentRequired,
			PaymentAmountMinor: output.CheckoutAction.PaymentAmountMinor,
		}
	}

	if output.InstallmentOffer != nil {
		offer := output.InstallmentOffer
		installments := make([]installmentOfferInstallmentResponse, 0, len(offer.Installments))
		for _, row := range offer.Installments {
			installments = append(installments, installmentOfferInstallmentResponse{
				ID:                   row.ID.UUID(),
				InstallmentOrder:     row.InstallmentOrder,
				Status:               row.Status,
				DueAt:                row.DueAt,
				PrincipalAmountMinor: row.PrincipalAmountMinor,
				UpfrontAmountMinor:   row.UpfrontAmountMinor,
				BaseFeeAmountMinor:   row.BaseFeeAmountMinor,
				TotalAmountMinor:     row.TotalAmountMinor,
			})
		}

		resp.InstallmentOffer = &installmentOfferResponse{
			InstallmentCount:        offer.InstallmentCount,
			PayableNowAmountMinor:   offer.PayableNowAmountMinor,
			TotalPayableAmountMinor: offer.TotalPayableAmountMinor,
			Currency:                offer.Currency,
			ExpiresAt:               offer.ExpiresAt,
			Installments:            installments,
		}
	}

	return resp
}

func toPurchaseContextResponse(context decisionuc.PurchaseContext) purchaseContextResponse {
	resp := purchaseContextResponse{
		Type:            context.Channel,
		MerchantOrderID: context.MerchantOrderID,
	}

	switch context.Channel {
	case model.POSPurchaseChannel:
		value := context.PosGpsID
		resp.PosGpsID = &value

	case model.OnlinePurchaseChannel:
		value := context.OnlineStoreID
		resp.OnlineStoreID = &value
	}

	return resp
}

// mapCheckoutDecisionOutcomeForResponse exposes only approved/denied on the
// checkout-decision HTTP API. Domain "conditional" (partial financing) is
// preserved in persistence and use-case logic, but presented as approved to
// API clients.
func mapCheckoutDecisionOutcomeForResponse(outcome model.DecisionOutcome) string {
	switch outcome {
	case model.ConditionalDecisionOutcome:
		return string(model.ApprovedDecisionOutcome)

	case model.ApprovedDecisionOutcome, model.DeniedDecisionOutcome:
		return string(outcome)

	default:
		// Defensive fallback for unexpected values; keep the raw outcome rather
		// than inventing approval/denial semantics.
		return string(outcome)
	}
}
