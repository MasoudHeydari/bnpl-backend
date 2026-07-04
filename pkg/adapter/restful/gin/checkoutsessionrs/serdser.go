package checkoutsessionrs

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/merchantterminal"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
)

// createPartnerRequest is the external merchant create body.
//
// There is deliberately no customer field. customer_reference is optional
// merchant correlation only. customer_phone is required; BNPL encrypts it and
// resolves it through Identity during Partner Create. Hosted OTP is a later
// possession/transaction-authorization step and does not establish the mapping.
//
// return_destination is a symbolic name registered during merchant onboarding.
// Arbitrary return URLs are never accepted.
type createPartnerRequest struct {
	OnlineStoreID   model.OnlineStoreID `json:"online_store_id" binding:"required" format:"uuid"`
	MerchantOrderID string              `json:"merchant_order_id" binding:"required,max=256" maxLength:"256"`
	// Distinct idempotency keys allow a new attempt after a terminal checkout.
	IdempotencyKey    string `json:"idempotency_key" binding:"max=256" maxLength:"256"`
	CustomerReference string `json:"customer_reference" binding:"max=256" maxLength:"256"`
	// CustomerPhone is the merchant-asserted customer phone. Its accepted format
	// is determined by MerchantProfile.Jurisdiction in Core. REST does not
	// TrimSpace or apply country-specific rules; max length is a generic bound.
	CustomerPhone string `json:"customer_phone" binding:"required,max=32" maxLength:"32" example:"+989121234567"`
	// ReturnDestination is a pre-registered lowercase kebab-case symbolic name
	// matching ^[a-z0-9]+(-[a-z0-9]+)*$ (1..64).
	// NOTE: swag v1.16 does not emit a machine-readable pattern field from tags;
	// the regex is documented here and enforced by Core/UseCase/DB.
	ReturnDestination string `json:"return_destination" binding:"required,max=64" maxLength:"64" minLength:"1"`
	// MerchantState is optional opaque correlation; empty is valid.
	MerchantState        string         `json:"merchant_state" binding:"max=256" maxLength:"256"`
	RequestedAmountMinor uint64         `json:"requested_amount_minor" binding:"required,min=1"`
	Currency             model.Currency `json:"currency" binding:"required"`
}

type partnerCreateResponse struct {
	// CheckoutSessionID is the public opaque token (cs_<64 hex>), not the
	// internal UUID. The JSON field name stays checkout_session_id for partners.
	CheckoutSessionID string                      `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	Status            model.CheckoutSessionStatus `json:"status" enums:"open,confirming,processing,completed,canceled,expired,unavailable"`
	CheckoutURL       string                      `json:"checkout_url"`
	ExpiresAt         time.Time                   `json:"expires_at"`
	// Offer is present only when the session's snapshotted branch
	// offer_preparation_mode is on_create. Omitted for on_start.
	Offer *OfferSchema `json:"offer,omitempty"`
}

// statusResponse is the authoritative merchant inquiry projection.
//
// It never includes customer credit, installment offer details, provider
// selection, or Financial Core payment identifiers.
type statusResponse struct {
	CheckoutSessionID string                      `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	MerchantOrderID   string                      `json:"merchant_order_id"`
	Status            model.CheckoutSessionStatus `json:"status" enums:"open,confirming,processing,completed,canceled,expired,unavailable"`
	AmountMinor       uint64                      `json:"amount_minor" example:"120000000"`
	Currency          model.Currency              `json:"currency" example:"IRR"`
	CompletedAt       *time.Time                  `json:"completed_at,omitempty"`
}

type cancelResponse struct {
	CheckoutSessionID string                      `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	Status            model.CheckoutSessionStatus `json:"status" enums:"canceled"`
	ReturnURL         string                      `json:"return_url"`
}

// confirmRequest carries the offer version (evaluation_generation) the customer
// actually saw. Provider/PSP selection is never accepted from the client.
type confirmRequest struct {
	// EvaluationGeneration is the offer version returned by start (or by a
	// previous reconfirm_required response). A stale value never creates a
	// contract or payment; Confirm returns a refreshed offer instead.
	EvaluationGeneration int `json:"evaluation_generation" binding:"required,min=1" example:"1"`
}

type confirmContractResponse struct {
	ID     uuid.UUID            `json:"id" example:"3fa85f64-5717-4562-b3fc-2c963f66afa6" format:"uuid"`
	Status model.ContractStatus `json:"status" example:"payment_pending"`
}

// confirmResponse is either a confirmed checkout (reconfirm_required false) or a
// reconfirm prompt (reconfirm_required true, refreshed offer, no contract).
//
// When payment_required is true, redirect_url and expires_at are the only
// customer-action continuation fields. Financial Core / PSP identifiers are
// never exposed on this public boundary.
type confirmResponse struct {
	CheckoutSessionID    uuid.UUID                   `json:"checkout_session_id" format:"uuid"`
	Status               model.CheckoutSessionStatus `json:"status" enums:"open,confirming,processing,completed,canceled,expired,unavailable"`
	ReconfirmRequired    bool                        `json:"reconfirm_required" example:"false"`
	EvaluationGeneration int                         `json:"evaluation_generation" example:"2"`
	Offer                *OfferSchema                `json:"offer,omitempty"`
	Contract             *confirmContractResponse    `json:"contract,omitempty"`
	PaymentRequired      bool                        `json:"payment_required" example:"true"`
	RedirectURL          string                      `json:"redirect_url,omitempty" example:"https://pay.example/session/abc"`
	ExpiresAt            string                      `json:"expires_at,omitempty" example:"2026-08-09T12:30:00Z"`
}

// returnResponse is the customer-facing browser-return projection.
//
// return_url carries only checkout_session_id and the opaque merchant state.
// Neither it nor merchant_state is payment truth; merchants confirm outcomes via
// the status endpoint or signed webhooks.
type returnResponse struct {
	CheckoutSessionID uuid.UUID                   `json:"checkout_session_id" format:"uuid"`
	Status            model.CheckoutSessionStatus `json:"status" enums:"open,confirming,processing,completed,canceled,expired,unavailable"`
	MerchantState     string                      `json:"merchant_state"`
	ReturnURL         string                      `json:"return_url"`
}

type upsertReturnDestinationRequest struct {
	ReturnURL string `json:"return_url" binding:"required,max=2048"`

	// Enabled is optional. Omit to preserve the stored value (true on create).
	Enabled *bool `json:"enabled,omitempty"`
}

type returnDestinationResponse struct {
	DestinationName string `json:"destination_name"`
	ReturnURL       string `json:"return_url"`
	Enabled         bool   `json:"enabled"`
}

type upsertWebhookDestinationRequest struct {
	WebhookURL string `json:"webhook_url" binding:"required,max=2048"`

	// SigningSecret is write-only. Omit to keep the stored secret; send a value
	// of at least 32 bytes to rotate it. Required on first registration.
	SigningSecret *string `json:"signing_secret,omitempty" binding:"omitempty,max=512" minLength:"32" maxLength:"512"`

	// Enabled is optional. Omit to preserve the stored value (true on create).
	Enabled *bool `json:"enabled,omitempty"`
}

type webhookDestinationResponse struct {
	WebhookURL string `json:"webhook_url"`
	Enabled    bool   `json:"enabled"`
}

func toCreatePartnerInput(c *gin.Context) (checkoutsessionuc.CreatePartnerInput, bool) {
	merchantID, ok := authenticatedMerchantID(c)
	if !ok {
		return checkoutsessionuc.CreatePartnerInput{}, false
	}

	var req createPartnerRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return checkoutsessionuc.CreatePartnerInput{}, false
	}

	if !req.Currency.IsValid() {
		serdser.SerValidationErr(c, "currency", "validation.currency_invalid")
		return checkoutsessionuc.CreatePartnerInput{}, false
	}

	return checkoutsessionuc.CreatePartnerInput{
		CheckoutSessionRequest: checkoutsessionuc.CheckoutSessionRequest{
			MerchantID:           merchantID,
			OnlineStoreID:        req.OnlineStoreID,
			MerchantOrderID:      strings.TrimSpace(req.MerchantOrderID),
			IdempotencyKey:       strings.TrimSpace(req.IdempotencyKey),
			CustomerReference:    strings.TrimSpace(req.CustomerReference),
			ReturnDestination:    req.ReturnDestination,
			MerchantState:        req.MerchantState,
			RequestedAmountMinor: req.RequestedAmountMinor,
			Currency:             req.Currency,
		},
		// Pass raw customer_phone unchanged — no TrimSpace, no country rules.
		CustomerPhone: req.CustomerPhone,
	}, true
}

func authenticatedMerchantID(c *gin.Context) (model.MerchantID, bool) {
	const op = "checkoutsessionrs.authenticatedMerchantID"

	// Partner terminal authentication is the source of truth for merchant
	// identity on partner routes.
	principal, ok := merchantterminal.MerchantPrincipalFromContext(c.Request.Context())
	if ok {
		return principal.MerchantID, true
	}

	// Routes that still use gateway authentication carry the merchant as the
	// subject.
	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return model.MerchantID{}, false
	}

	return authCtx.SubjectID.AsMerchantID(), true
}

func authenticatedExternalCustomerID(c *gin.Context) (model.ExternalCustomerID, bool) {
	const op = "checkoutsessionrs.authenticatedExternalCustomerID"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return model.ExternalCustomerID{}, false
	}

	return authCtx.SubjectID.AsExternalCustomerID(), true
}

func checkoutSessionIDParam(c *gin.Context) (model.CheckoutSessionID, bool) {
	id, err := model.ParseCheckoutSessionID(c.Param("checkout_session_id"))
	if err != nil || id.IsZero() {
		serdser.SerValidationErr(c, "checkout_session_id", "validation.checkout_session_id_invalid")
		return model.CheckoutSessionID{}, false
	}

	return id, true
}

// checkoutSessionTokenParam parses the merchant/browser-facing public token
// from the checkout_session_id path parameter.
func checkoutSessionTokenParam(c *gin.Context) (model.CheckoutSessionToken, bool) {
	token, err := model.ParseCheckoutSessionToken(c.Param("checkout_session_id"))
	if err != nil || token.IsZero() {
		serdser.SerValidationErr(c, "checkout_session_id", "validation.checkout_session_id_invalid")
		return "", false
	}

	return token, true
}

// checkoutTokenParam parses the customer-facing public token from :token.
func checkoutTokenParam(c *gin.Context) (model.CheckoutSessionToken, bool) {
	token, err := model.ParseCheckoutSessionToken(c.Param("token"))
	if err != nil || token.IsZero() {
		serdser.SerValidationErr(c, "token", "validation.checkout_session_id_invalid")
		return "", false
	}

	return token, true
}

func toPartnerCreateResponse(output checkoutsessionuc.CreatePartnerOutput) partnerCreateResponse {
	resp := partnerCreateResponse{
		CheckoutSessionID: output.Session.PublicToken.String(),
		Status:            output.Session.Status,
		CheckoutURL:       output.CheckoutURL,
		ExpiresAt:         output.Session.ExpiresAt,
	}

	if output.Offer != nil {
		offer := toOfferResponse(*output.Offer)
		resp.Offer = &offer
	}

	return resp
}

func toOfferResponse(offer checkoutsessionuc.Offer) OfferSchema {
	// A denied offer exposes only the outcome. Financing fields stay absent so a
	// zero never reads as "financed nothing, owes nothing".
	if offer.Outcome != checkoutsessionuc.ApprovedOfferOutcome {
		return OfferSchema{Outcome: offer.Outcome}
	}

	installments := make([]OfferInstallmentSchema, 0, len(offer.Installments))
	for _, row := range offer.Installments {
		installments = append(installments, OfferInstallmentSchema{
			InstallmentOrder: row.InstallmentOrder,
			DueAt:            row.DueAt,
			TotalAmountMinor: row.TotalAmountMinor,
		})
	}

	resp := OfferSchema{
		Outcome:                        offer.Outcome,
		AvailableCreditMinor:           &offer.AvailableCreditMinor,
		RequestedAmountMinor:           &offer.RequestedAmountMinor,
		ApprovedFinancedAmountMinor:    &offer.ApprovedFinancedAmountMinor,
		RequiredDownPaymentAmountMinor: &offer.RequiredDownPaymentAmountMinor,
		Currency:                       offer.Currency,
		OfferExpiresAt:                 offer.OfferExpiresAt,
		InstallmentCount:               &offer.InstallmentCount,
		PayableNowAmountMinor:          &offer.PayableNowAmountMinor,
		TotalPayableAmountMinor:        &offer.TotalPayableAmountMinor,
		Installments:                   installments,
	}

	if offer.CheckoutAction != nil {
		action := CheckoutActionSchema{
			Type:               offer.CheckoutAction.Type,
			PaymentRequired:    offer.CheckoutAction.PaymentRequired,
			PaymentAmountMinor: offer.CheckoutAction.PaymentAmountMinor,
		}
		resp.CheckoutAction = &action
	}

	return resp
}

func toStatusResponse(output checkoutsessionuc.GetPartnerStatusOutput) statusResponse {
	return statusResponse{
		CheckoutSessionID: output.CheckoutSessionToken.String(),
		MerchantOrderID:   output.MerchantOrderID,
		Status:            output.Status,
		AmountMinor:       output.AmountMinor,
		Currency:          output.Currency,
		CompletedAt:       output.CompletedAt,
	}
}

func toCancelResponse(output checkoutsessionuc.CancelOutput) cancelResponse {
	return cancelResponse{
		CheckoutSessionID: output.Session.PublicToken.String(),
		Status:            output.Session.Status,
		ReturnURL:         output.ReturnURL,
	}
}

func toConfirmResponse(output checkoutsessionuc.ConfirmOutput) confirmResponse {
	response := confirmResponse{
		CheckoutSessionID:    output.Session.ID.UUID(),
		Status:               output.Session.Status,
		EvaluationGeneration: output.Session.EvaluationGeneration,
	}

	if output.ReconfirmRequired {
		offer := toOfferResponse(output.Offer)
		response.ReconfirmRequired = true
		response.Offer = &offer

		return response
	}

	response.Contract = &confirmContractResponse{
		ID:     output.Contract.ID.UUID(),
		Status: output.Contract.Status,
	}
	response.PaymentRequired = output.PaymentRequired

	if output.PaymentRequired {
		response.RedirectURL = strings.TrimSpace(output.Payment.PaymentLink)
		response.ExpiresAt = strings.TrimSpace(output.Payment.ExpiresAt)
	}

	return response
}

func toUpsertReturnDestinationInput(c *gin.Context) (checkoutsessionuc.UpsertReturnDestinationInput, bool) {
	merchantID, ok := authenticatedMerchantID(c)
	if !ok {
		return checkoutsessionuc.UpsertReturnDestinationInput{}, false
	}

	destinationName, ok := destinationNameParam(c)
	if !ok {
		return checkoutsessionuc.UpsertReturnDestinationInput{}, false
	}

	var req upsertReturnDestinationRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return checkoutsessionuc.UpsertReturnDestinationInput{}, false
	}

	return checkoutsessionuc.UpsertReturnDestinationInput{
		MerchantID:      merchantID,
		DestinationName: destinationName,
		// Pass the raw URL unchanged. Leading/trailing whitespace must be rejected
		// by Core policy, not silently normalized at the REST boundary.
		ReturnURL: req.ReturnURL,
		Enabled:   req.Enabled,
	}, true
}

func toReturnDestinationResponse(
	output checkoutsessionuc.UpsertReturnDestinationOutput,
) returnDestinationResponse {
	return returnDestinationResponse{
		DestinationName: output.Destination.DestinationName,
		ReturnURL:       output.Destination.ReturnURL,
		Enabled:         output.Destination.Enabled,
	}
}

func toUpsertWebhookDestinationInput(c *gin.Context) (checkoutsessionuc.UpsertWebhookDestinationInput, bool) {
	merchantID, ok := authenticatedMerchantID(c)
	if !ok {
		return checkoutsessionuc.UpsertWebhookDestinationInput{}, false
	}

	var req upsertWebhookDestinationRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return checkoutsessionuc.UpsertWebhookDestinationInput{}, false
	}

	return checkoutsessionuc.UpsertWebhookDestinationInput{
		MerchantID:    merchantID,
		WebhookURL:    strings.TrimSpace(req.WebhookURL),
		SigningSecret: req.SigningSecret,
		Enabled:       req.Enabled,
	}, true
}

func toWebhookDestinationResponse(
	output checkoutsessionuc.UpsertWebhookDestinationOutput,
) webhookDestinationResponse {
	return webhookDestinationResponse{
		WebhookURL: output.Destination.WebhookURL,
		Enabled:    output.Destination.Enabled,
	}
}

func destinationNameParam(c *gin.Context) (string, bool) {
	// Path identity is validated as-is (no trim/lowercase rewriting) using the
	// shared Core grammar so Gin and UseCase reject the same invalid names.
	name := c.Param("destination_name")
	if err := returndestination.ValidateDestinationName(name); err != nil {
		serdser.SerValidationErr(c, "destination_name", "validation.destination_name_invalid")
		return "", false
	}

	return name, true
}
