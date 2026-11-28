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
	ExternalOnlineStoreID model.ExternalOnlineStoreID `json:"external_online_store_id" binding:"required" format:"uuid"`
	MerchantOrderID       string                      `json:"merchant_order_id" binding:"required,max=256" maxLength:"256"`
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
	// EvaluationGeneration is the optimistic-concurrency token Partner must
	// echo on SelectCreditKind (PUT .../credit-kind), including the first
	// selection. Create starts at 1 and does not prepare offers.
	EvaluationGeneration int       `json:"evaluation_generation" example:"1"`
	ExpiresAt            time.Time `json:"expires_at"`
	// Offer is present only after Partner SelectCreditKind when preparation
	// mode is on_create (Create itself does not prepare offers).
	Offer *OfferSchema `json:"offer,omitempty"`
	// CreditKinds is the dual-kind eligibility preview for the merchant UI.
	CreditKinds []creditKindPreviewSchema `json:"credit_kinds"`
}

type creditKindPreviewSchema struct {
	Kind             model.CreditKind               `json:"kind" enums:"pay_in_installments,pay_next_month"`
	Available        bool                           `json:"available" example:"true"`
	Reason           *creditKindPreviewReasonSchema `json:"reason,omitempty"`
	RepaymentPlan    model.RepaymentPlanKind        `json:"repayment_plan,omitempty" enums:"pay_in_3,pay_in_4"`
	InstallmentCount int                            `json:"installment_count,omitempty" example:"3"`
}

type creditKindPreviewReasonSchema struct {
	Code    string `json:"code" example:"CREDIT_KIND_DISABLED"`
	Message string `json:"message" example:"This merchant does not offer the selected credit kind." i18n:"localized"`
}

type selectCreditKindRequest struct {
	CreditKind model.CreditKind `json:"credit_kind" binding:"required" enums:"pay_in_installments,pay_next_month"`
	// ExpectedEvaluationGeneration fences concurrent / delayed PUT retries.
	// Required for every selection mutation, including the first selection
	// (echo evaluation_generation from Partner Create).
	ExpectedEvaluationGeneration int `json:"expected_evaluation_generation" binding:"required,min=1" example:"1"`
}

type selectCreditKindResponse struct {
	CheckoutSessionID string                      `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	Status            model.CheckoutSessionStatus `json:"status" enums:"open,confirming,processing,completed,canceled,expired,unavailable"`
	CreditKind        model.CreditKind            `json:"credit_kind" enums:"pay_in_installments,pay_next_month"`
	// EvaluationGeneration is the authoritative generation after selection.
	// Echo it on a subsequent replacement PUT as expected_evaluation_generation.
	EvaluationGeneration int          `json:"evaluation_generation" example:"1"`
	ExpiresAt            time.Time    `json:"expires_at"`
	Offer                *OfferSchema `json:"offer,omitempty"`
}

// statusResponse is the authoritative merchant inquiry projection.
//
// It never includes customer credit, installment offer details, provider
// selection, or Financial Core payment identifiers.
type statusResponse struct {
	CheckoutSessionID      string                      `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	MerchantOrderID        string                      `json:"merchant_order_id"`
	Status                 model.CheckoutSessionStatus `json:"status" enums:"open,confirming,processing,verify_pending,verified_by_merchant,cancellation_pending,canceled,expired,unavailable"`
	AmountMinor            uint64                      `json:"amount_minor" example:"120000000"`
	Currency               model.Currency              `json:"currency" example:"IRR"`
	VerificationDeadlineAt *time.Time                  `json:"verification_deadline_at,omitempty"`
	VerifiedAt             *time.Time                  `json:"verified_at,omitempty"`
}

type cancelResponse struct {
	CheckoutSessionID string                      `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	Status            model.CheckoutSessionStatus `json:"status" enums:"canceled"`
	ReturnURL         string                      `json:"return_url"`
}

// verifyByMerchantRequest is the partner Verify body.
type verifyByMerchantRequest struct {
	MerchantOrderID string `json:"merchant_order_id" binding:"required,max=256" maxLength:"256"`
}

type verifyByMerchantResponse struct {
	CheckoutSessionID string                      `json:"checkout_session_id" example:"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`
	MerchantOrderID   string                      `json:"merchant_order_id"`
	Status            model.CheckoutSessionStatus `json:"status" enums:"verified_by_merchant"`
	VerifiedAt        time.Time                   `json:"verified_at"`
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
// the status endpoint.
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
			ExternalMerchantID:    merchantID,
			ExternalOnlineStoreID: req.ExternalOnlineStoreID,
			MerchantOrderID:       strings.TrimSpace(req.MerchantOrderID),
			IdempotencyKey:        strings.TrimSpace(req.IdempotencyKey),
			CustomerReference:     strings.TrimSpace(req.CustomerReference),
			ReturnDestination:     req.ReturnDestination,
			MerchantState:         req.MerchantState,
			RequestedAmountMinor:  req.RequestedAmountMinor,
			Currency:              req.Currency,
		},
		// Pass raw customer_phone unchanged — no TrimSpace, no country rules.
		CustomerPhone: req.CustomerPhone,
	}, true
}

func authenticatedMerchantID(c *gin.Context) (model.ExternalMerchantID, bool) {
	const op = "checkoutsessionrs.authenticatedMerchantID"

	// Partner terminal authentication is the source of truth for merchant
	// identity on partner routes.
	principal, ok := merchantterminal.MerchantPrincipalFromContext(c.Request.Context())
	if ok {
		return principal.ExternalMerchantID, true
	}

	serdser.SerErr(c, cerr.Unauthorized(
		fmt.Errorf("%s: terminal principal is missing", op),
	))

	return model.ExternalMerchantID{}, false
}

func authenticatedExternalSubjectID(c *gin.Context) (model.ExternalSubjectID, bool) {
	const op = "checkoutsessionrs.authenticatedExternalSubjectID"

	authCtx, ok := permission.FromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authentication context is missing", op),
		))
		return model.ExternalSubjectID{}, false
	}

	return authCtx.SubjectID.AsExternalSubjectID(), true
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
		CheckoutSessionID:    output.Session.PublicToken.String(),
		Status:               output.Session.Status,
		CheckoutURL:          output.CheckoutURL,
		EvaluationGeneration: output.Session.EvaluationGeneration,
		ExpiresAt:            output.Session.ExpiresAt,
		CreditKinds:          toCreditKindPreviews(output.CreditKinds),
	}

	if output.Offer != nil {
		offer := toOfferResponse(*output.Offer)
		resp.Offer = &offer
	}

	return resp
}

func toCreditKindPreviews(previews []checkoutsessionuc.CreditKindPreview) []creditKindPreviewSchema {
	out := make([]creditKindPreviewSchema, 0, len(previews))
	for _, preview := range previews {
		item := creditKindPreviewSchema{
			Kind:             preview.Kind,
			Available:        preview.Available,
			RepaymentPlan:    preview.RepaymentPlan,
			InstallmentCount: preview.InstallmentCount,
		}
		if preview.Reason != nil {
			item.Reason = &creditKindPreviewReasonSchema{
				Code:    preview.Reason.Code,
				Message: preview.Reason.MessageKey,
			}
		}

		out = append(out, item)
	}

	return out
}

func toSelectCreditKindResponse(output checkoutsessionuc.SelectCreditKindOutput) selectCreditKindResponse {
	resp := selectCreditKindResponse{
		CheckoutSessionID:    output.Session.PublicToken.String(),
		Status:               output.Session.Status,
		CreditKind:           output.Session.CreditKind,
		EvaluationGeneration: output.Session.EvaluationGeneration,
		ExpiresAt:            output.Session.ExpiresAt,
	}

	if output.Offer != nil {
		offer := toOfferResponse(*output.Offer)
		resp.Offer = &offer
	}

	return resp
}

func toOfferResponse(offer checkoutsessionuc.Offer) OfferSchema {
	// A denied offer normally exposes only the outcome. Capacity shortfalls may
	// additionally include available_credit_minor so Partner UIs can explain
	// insufficient credit without leaking policy-denial balances.
	if offer.Outcome != checkoutsessionuc.ApprovedOfferOutcome {
		resp := OfferSchema{Outcome: offer.Outcome}
		if offer.ExposeAvailableCredit {
			credit := offer.AvailableCreditMinor
			resp.AvailableCreditMinor = &credit
		}

		return resp
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
		CreditKind:                     offer.CreditKind,
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

	if offer.RepaymentPlan != "" {
		plan := offer.RepaymentPlan
		resp.RepaymentPlan = &plan
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
		CheckoutSessionID:      output.CheckoutSessionToken.String(),
		MerchantOrderID:        output.MerchantOrderID,
		Status:                 output.Status,
		AmountMinor:            output.AmountMinor,
		Currency:               output.Currency,
		VerificationDeadlineAt: output.VerificationDeadlineAt,
		VerifiedAt:             output.VerifiedAt,
	}
}

func toVerifyByMerchantInput(c *gin.Context) (checkoutsessionuc.VerifyByMerchantInput, bool) {
	const op = "checkoutsessionrs.toVerifyByMerchantInput"

	token, ok := checkoutSessionTokenParam(c)
	if !ok {
		return checkoutsessionuc.VerifyByMerchantInput{}, false
	}

	principal, ok := merchantterminal.MerchantPrincipalFromContext(c.Request.Context())
	if !ok {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: partner terminal principal is missing", op),
		))
		return checkoutsessionuc.VerifyByMerchantInput{}, false
	}

	if principal.MerchantTerminalID.IsZero() {
		serdser.SerErr(c, cerr.Unauthorized(
			fmt.Errorf("%s: authenticated terminal id is missing", op),
		))
		return checkoutsessionuc.VerifyByMerchantInput{}, false
	}

	var req verifyByMerchantRequest
	if !serdser.Bind(c, &req, binding.JSON) {
		return checkoutsessionuc.VerifyByMerchantInput{}, false
	}

	return checkoutsessionuc.VerifyByMerchantInput{
		CheckoutSessionToken: token,
		ExternalMerchantID:   principal.ExternalMerchantID,
		MerchantOrderID:      req.MerchantOrderID,
		TerminalID:           principal.MerchantTerminalID,
	}, true
}

func toVerifyByMerchantResponse(
	output checkoutsessionuc.VerifyByMerchantOutput,
) verifyByMerchantResponse {
	return verifyByMerchantResponse{
		CheckoutSessionID: output.CheckoutSessionToken.String(),
		MerchantOrderID:   output.MerchantOrderID,
		Status:            output.Status,
		VerifiedAt:        output.VerifiedAt.UTC(),
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
		ExternalMerchantID: merchantID,
		DestinationName:    destinationName,
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
