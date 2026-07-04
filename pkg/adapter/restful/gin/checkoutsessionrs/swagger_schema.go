package checkoutsessionrs

import (
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

// CreatePartnerRequestSchema is a schema-only representation of the external
// partner create body. It matches createPartnerRequest JSON exactly and contains
// no business logic.
type CreatePartnerRequestSchema createPartnerRequest

// PartnerCreateResponseSchema is a schema-only representation of the partner
// create response data.
type PartnerCreateResponseSchema partnerCreateResponse

// PartnerCreateSuccessSchema is a schema-only success envelope for partner create.
type PartnerCreateSuccessSchema struct {
	Data PartnerCreateResponseSchema     `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// CheckoutActionSchema is the swagger-stable checkout action projection.
type CheckoutActionSchema struct {
	Type               decisionuc.CheckoutActionType `json:"type" enums:"confirm_and_pay,confirm_purchase"`
	PaymentRequired    bool                          `json:"payment_required"`
	PaymentAmountMinor uint64                        `json:"payment_amount_minor"`
}

// OfferInstallmentSchema is the swagger-stable installment projection.
type OfferInstallmentSchema struct {
	InstallmentOrder int       `json:"installment_order"`
	DueAt            time.Time `json:"due_at"`
	TotalAmountMinor uint64    `json:"total_amount_minor"`
}

// OfferSchema is the swagger-stable financing offer projection.
//
// A denied offer carries only outcome; every financing field is omitted.
type OfferSchema struct {
	Outcome                        checkoutsessionuc.OfferOutcome `json:"outcome" enums:"approved,denied"`
	AvailableCreditMinor           *uint64                        `json:"available_credit_minor,omitempty"`
	RequestedAmountMinor           *uint64                        `json:"requested_amount_minor,omitempty"`
	ApprovedFinancedAmountMinor    *uint64                        `json:"approved_financed_amount_minor,omitempty"`
	RequiredDownPaymentAmountMinor *uint64                        `json:"required_down_payment_amount_minor,omitempty"`
	Currency                       model.Currency                 `json:"currency,omitempty"`
	OfferExpiresAt                 *time.Time                     `json:"offer_expires_at,omitempty"`
	InstallmentCount               *int                           `json:"installment_count,omitempty"`
	PayableNowAmountMinor          *uint64                        `json:"payable_now_amount_minor,omitempty"`
	TotalPayableAmountMinor        *uint64                        `json:"total_payable_amount_minor,omitempty"`
	Installments                   []OfferInstallmentSchema       `json:"installments,omitempty"`
	CheckoutAction                 *CheckoutActionSchema          `json:"checkout_action,omitempty"`
}

// StatusResponseSchema is a schema-only representation of merchant-safe checkout state.
type StatusResponseSchema statusResponse

// StatusSuccessSchema is a schema-only success envelope for merchant status polling.
type StatusSuccessSchema struct {
	Data StatusResponseSchema            `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// CancelResponseSchema is a schema-only representation of the cancel response data.
type CancelResponseSchema cancelResponse

// CancelSuccessSchema is a schema-only success envelope for checkout cancellation.
type CancelSuccessSchema struct {
	Data CancelResponseSchema            `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// ConfirmRequestSchema is a schema-only Confirm & Pay request body.
type ConfirmRequestSchema confirmRequest

// ConfirmContractSchema is a schema-only contract projection in Confirm responses.
type ConfirmContractSchema confirmContractResponse

// ConfirmResponseSchema is a schema-only Confirm & Pay response data.
type ConfirmResponseSchema confirmResponse

// ConfirmSuccessSchema is a schema-only success envelope for Confirm & Pay.
type ConfirmSuccessSchema struct {
	Data ConfirmResponseSchema           `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// ReturnResponseSchema is a schema-only customer browser-return projection.
type ReturnResponseSchema returnResponse

// ReturnSuccessSchema is a schema-only success envelope for the customer return projection.
type ReturnSuccessSchema struct {
	Data ReturnResponseSchema            `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// UpsertReturnDestinationRequestSchema is a schema-only return destination body.
type UpsertReturnDestinationRequestSchema upsertReturnDestinationRequest

// ReturnDestinationResponseSchema is a schema-only return destination response.
type ReturnDestinationResponseSchema returnDestinationResponse

// ReturnDestinationSuccessSchema is a schema-only success envelope for return destinations.
type ReturnDestinationSuccessSchema struct {
	Data ReturnDestinationResponseSchema `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// UpsertWebhookDestinationRequestSchema is a schema-only webhook destination body.
type UpsertWebhookDestinationRequestSchema upsertWebhookDestinationRequest

// WebhookDestinationResponseSchema is a schema-only webhook destination response.
type WebhookDestinationResponseSchema webhookDestinationResponse

// WebhookDestinationSuccessSchema is a schema-only success envelope for webhook destinations.
type WebhookDestinationSuccessSchema struct {
	Data WebhookDestinationResponseSchema `json:"data"`
	Meta serdser.SuccessMetaNoPagination  `json:"meta"`
}

// HostedExternalGetResponseSchema is a schema-only hosted external page projection.
type HostedExternalGetResponseSchema hostedExternalGetResponse

// HostedExternalGetSuccessSchema is a schema-only success envelope for the hosted external GET.
type HostedExternalGetSuccessSchema struct {
	Data HostedExternalGetResponseSchema `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}

// HostedExternalStartRequestSchema is a schema-only hosted external start body.
type HostedExternalStartRequestSchema hostedExternalStartRequest

// HostedExternalStartResponseSchema is a schema-only hosted external start response.
type HostedExternalStartResponseSchema hostedExternalStartResponse

// HostedExternalStartSuccessSchema is a schema-only success envelope for the hosted external start.
type HostedExternalStartSuccessSchema struct {
	Data HostedExternalStartResponseSchema `json:"data"`
	Meta serdser.SuccessMetaNoPagination   `json:"meta"`
}

// HostedExternalConfirmRequestSchema is a schema-only hosted external confirm body.
type HostedExternalConfirmRequestSchema hostedExternalConfirmRequest

// HostedExternalConfirmResponseSchema is a schema-only hosted external confirm response.
type HostedExternalConfirmResponseSchema hostedExternalConfirmResponse

// HostedExternalConfirmSuccessSchema is a schema-only success envelope for the hosted external confirm.
type HostedExternalConfirmSuccessSchema struct {
	Data HostedExternalConfirmResponseSchema `json:"data"`
	Meta serdser.SuccessMetaNoPagination     `json:"meta"`
}

// HostedConfirmationStatusResponseSchema is a schema-only hosted confirmation-status projection.
type HostedConfirmationStatusResponseSchema hostedConfirmationStatusResponse

// HostedConfirmationStatusSuccessSchema is a schema-only success envelope for
// the hosted confirmation-status poll.
type HostedConfirmationStatusSuccessSchema struct {
	Data HostedConfirmationStatusResponseSchema `json:"data"`
	Meta serdser.SuccessMetaNoPagination        `json:"meta"`
}

// HostedExternalOTPRequestSchema is a schema-only hosted OTP request body.
type HostedExternalOTPRequestSchema hostedExternalOTPRequest

// HostedExternalOTPResponseSchema is a schema-only hosted OTP response.
type HostedExternalOTPResponseSchema hostedExternalOTPResponse

// HostedExternalOTPSuccessSchema is a schema-only success envelope for hosted OTP issuance.
type HostedExternalOTPSuccessSchema struct {
	Data HostedExternalOTPResponseSchema `json:"data"`
	Meta serdser.SuccessMetaNoPagination `json:"meta"`
}
