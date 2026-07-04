package checkoutsessionuc

import (
	"fmt"
	"math"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/checkoutotp"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	returndestination "github.com/parstis/bnpl-backend/pkg/core/policy/returndestination"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/decisionuc"
)

// CreateResult identifies how a checkout session create completed.
type CreateResult string

const (
	// CreateResultCreated indicates a new checkout session was persisted.
	CreateResultCreated CreateResult = "created"

	// CreateResultAlreadyExists indicates an idempotent replay returned the existing session.
	CreateResultAlreadyExists CreateResult = "already_exists"
)

// OfferOutcome is the merchant/customer-facing financing result.
//
// Persisted domain outcomes include a conditional (partial financing) value.
// It is presented as approved here; risk reason codes are never projected.
type OfferOutcome string

const (
	// ApprovedOfferOutcome indicates BNPL can finance the checkout.
	ApprovedOfferOutcome OfferOutcome = "approved"

	// DeniedOfferOutcome indicates BNPL cannot finance the checkout.
	DeniedOfferOutcome OfferOutcome = "denied"
)

// CheckoutSessionRequest carries the merchant-supplied terms shared by the
// external and internal create paths.
type CheckoutSessionRequest struct {
	// MerchantID is the authenticated merchant identity.
	MerchantID model.MerchantID

	// OnlineStoreID addresses the merchant storefront this checkout belongs to.
	OnlineStoreID model.OnlineStoreID

	// MerchantOrderID is the merchant-owned business order reference.
	MerchantOrderID string

	// IdempotencyKey identifies this create request for the merchant. When empty
	// it defaults to MerchantOrderID for backward compatibility during the
	// transition to caller-supplied keys.
	IdempotencyKey string

	// CustomerReference is optional merchant-owned correlation data. It is
	// never used to resolve a customer identity.
	CustomerReference string

	// ReturnDestination names a pre-registered merchant return destination.
	ReturnDestination string

	// MerchantState is opaque merchant correlation data echoed on browser return.
	MerchantState string

	// RequestedAmountMinor is the checkout amount in minor currency units.
	RequestedAmountMinor uint64

	// Currency is the currency of RequestedAmountMinor.
	Currency model.Currency
}

// EffectiveIdempotencyKey returns the create idempotency key, falling back to
// MerchantOrderID when the caller did not supply one.
func (r CheckoutSessionRequest) EffectiveIdempotencyKey() string {
	if r.IdempotencyKey != "" {
		return r.IdempotencyKey
	}

	return r.MerchantOrderID
}

// CreatePartnerInput carries an external merchant hosted-handoff request.
//
// It deliberately has no customer field. An external merchant can never name a
// BNPL customer; Partner Create resolves the Merchant-asserted verified phone
// through Identity and prepares the offer. Hosted OTP happens later and proves
// possession / authorizes that exact offer generation.
type CreatePartnerInput struct {
	CheckoutSessionRequest

	// CustomerPhone is the raw merchant-asserted customer phone. Accepted format
	// is determined by MerchantProfile.Jurisdiction after the merchant profile
	// is loaded; this field is not yet canonical. It is canonicalized, then
	// encrypted at rest, and later proven by OTP on the hosted page.
	CustomerPhone string

	// TenantID is the trusted Gateway tenant scope for this Partner request.
	// It is snapshotted onto the CheckoutSession and later used by Hosted
	// Confirm. It must never come from request JSON.
	TenantID model.TenantID

	// ContextID is the trusted Gateway business context for this Partner
	// request. It is snapshotted onto the CheckoutSession and later used by
	// Hosted Confirm. It must never come from request JSON.
	ContextID model.ContextID
}

// CreatePartnerOutput returns the merchant-safe hosted handoff.
type CreatePartnerOutput struct {
	// Result reports whether the session was created or replayed.
	Result CreateResult

	// Session is the persisted checkout session.
	Session model.CheckoutSession

	// CheckoutURL is the hosted BNPL page the merchant redirects the browser to.
	CheckoutURL string
}

// CreateInternalInput carries a trusted first-party create request.
//
// IdentitySubjectID is the Identity Service subject the trusted caller acts for.
type CreateInternalInput struct {
	CheckoutSessionRequest

	// IdentitySubjectID is the trusted customer identity to evaluate immediately.
	IdentitySubjectID model.IdentitySubjectID
}

// CreateInternalOutput returns the session, hosted URL, and offer economics.
type CreateInternalOutput struct {
	// Result reports whether the session was created or replayed.
	Result CreateResult

	// Session is the persisted checkout session.
	Session model.CheckoutSession

	// CheckoutURL is the hosted BNPL page the merchant redirects the browser to
	// for final customer confirmation.
	CheckoutURL string

	// Offer is the financing projection for the trusted customer.
	Offer Offer
}

// StartInput carries an authenticated customer's request to evaluate a checkout.
type StartInput struct {
	// CheckoutSessionID identifies the hosted checkout being started.
	CheckoutSessionID model.CheckoutSessionID

	// IdentitySubjectID is the authenticated customer identity.
	IdentitySubjectID model.IdentitySubjectID
}

// StartOutput returns the evaluated checkout state and offer.
type StartOutput struct {
	// Session is the checkout session after evaluation.
	Session model.CheckoutSession

	// Offer is the financing projection for the bound customer.
	Offer Offer
}

// GetPartnerStatusInput carries a merchant status poll.
type GetPartnerStatusInput struct {
	// CheckoutSessionToken is the public token of the checkout being polled.
	CheckoutSessionToken model.CheckoutSessionToken

	// MerchantID is the authenticated merchant identity.
	MerchantID model.MerchantID
}

// GetPartnerStatusOutput is the authoritative merchant inquiry projection.
//
// It never includes customer credit, installment offers, provider selection,
// or Financial Core payment identifiers.
type GetPartnerStatusOutput struct {
	// CheckoutSessionToken is the merchant-visible checkout identifier.
	CheckoutSessionToken model.CheckoutSessionToken

	// MerchantOrderID is the merchant-owned order reference.
	MerchantOrderID string

	// Status is the effective merchant-facing checkout status. Open sessions
	// past expires_at are projected as expired without a write.
	Status model.CheckoutSessionStatus

	// AmountMinor is the checkout requested amount in minor units.
	AmountMinor uint64

	// Currency is the checkout currency.
	Currency model.Currency

	// CompletedAt is set only when Status is completed.
	CompletedAt *time.Time
}

// CancelByMerchantInput carries a merchant cancellation.
type CancelByMerchantInput struct {
	// CheckoutSessionToken is the public token of the checkout being canceled.
	CheckoutSessionToken model.CheckoutSessionToken

	// MerchantID is the authenticated merchant identity.
	MerchantID model.MerchantID
}

// CancelByCustomerInput carries an explicit customer cancellation.
type CancelByCustomerInput struct {
	// CheckoutSessionID identifies the checkout being canceled.
	CheckoutSessionID model.CheckoutSessionID

	// IdentitySubjectID is the authenticated customer identity.
	IdentitySubjectID model.IdentitySubjectID
}

// CancelOutput returns the canceled session and the browser return target.
type CancelOutput struct {
	// Session is the canceled checkout session.
	Session model.CheckoutSession

	// ReturnURL is the pre-registered merchant URL with the checkout id and merchant state.
	ReturnURL string
}

// ConfirmInput carries an authenticated customer's Confirm & Pay request.
//
// Decision and Contract identifiers are never supplied by the client; Confirm
// reads them from the CheckoutSession. EvaluationGeneration is the offer version
// the customer actually saw (returned by /start) and must match the session.
type ConfirmInput struct {
	// CheckoutSessionID identifies the hosted checkout being confirmed.
	CheckoutSessionID model.CheckoutSessionID

	// IdentitySubjectID is the authenticated customer identity.
	IdentitySubjectID model.IdentitySubjectID

	// TenantID identifies the tenant for the payment context.
	TenantID model.TenantID

	// ContextID identifies the payment context.
	ContextID model.ContextID

	// EvaluationGeneration is the offer/evaluation version presented to the customer.
	EvaluationGeneration int
}

// ConfirmOutput returns the linked contract and payment continuation data, or a
// reconfirm payload when the presented offer version is stale.
type ConfirmOutput struct {
	// ReconfirmRequired is true when the customer confirmed a stale offer.
	// No Contract is created; Offer holds the refreshed terms to present.
	ReconfirmRequired bool

	// Session is the checkout session after the confirm attempt.
	Session model.CheckoutSession

	// Offer is set when ReconfirmRequired is true.
	Offer Offer

	// Contract is the created or idempotently reused contract.
	Contract model.Contract

	// PaymentRequired reports whether the customer must complete a customer
	// payment action before the checkout can complete.
	PaymentRequired bool

	// PaymentAttempt is present when PaymentRequired is true.
	PaymentAttempt model.PaymentAttempt

	// Payment carries internal payment continuation details used by the REST
	// adapter to project redirect_url and expires_at. Callers must not expose
	// Financial Core identifiers from this value on public boundaries.
	Payment contractuc.CheckoutPaymentOutput
}

// GetReturnInput carries a customer's request for the browser-return projection.
type GetReturnInput struct {
	// CheckoutSessionID identifies the checkout the customer is returning from.
	CheckoutSessionID model.CheckoutSessionID

	// IdentitySubjectID is the authenticated customer identity.
	IdentitySubjectID model.IdentitySubjectID
}

// GetReturnOutput is the customer-facing browser-return projection.
//
// ReturnURL carries only checkout_session_id and the opaque merchant state.
// It never encodes payment truth; merchants must confirm outcomes through the
// status endpoint or signed webhooks.
type GetReturnOutput struct {
	// Session is the checkout session the customer is bound to.
	Session model.CheckoutSession

	// ReturnURL is the pre-registered merchant URL with checkout_session_id and state.
	ReturnURL string
}

// ExpireDueOpenSessionsOutput reports how many open sessions one expiry sweep closed.
type ExpireDueOpenSessionsOutput struct {
	// ExpiredCount is the number of sessions transitioned to expired.
	ExpiredCount int
}

// UpsertReturnDestinationInput registers a browser return destination.
type UpsertReturnDestinationInput struct {
	MerchantID      model.MerchantID
	DestinationName string
	ReturnURL       string

	// Enabled is optional. Nil preserves the stored value on update and defaults
	// to true on create, so omitting it never silently disables a destination.
	Enabled *bool
}

// UpsertReturnDestinationOutput returns the persisted return destination.
type UpsertReturnDestinationOutput struct {
	Destination model.MerchantReturnDestination
}

// UpsertWebhookDestinationInput registers the merchant's single lifecycle
// webhook endpoint.
type UpsertWebhookDestinationInput struct {
	MerchantID model.MerchantID
	WebhookURL string

	// SigningSecret is write-only. Nil preserves the stored secret; a non-nil
	// value rotates it. The first registration must supply a secret of at least
	// model.MinMerchantWebhookSigningSecretLength bytes.
	SigningSecret *string

	// Enabled is optional. Nil preserves the stored value on update and defaults
	// to true on create.
	Enabled *bool
}

// UpsertWebhookDestinationOutput returns the persisted webhook destination.
type UpsertWebhookDestinationOutput struct {
	Destination model.MerchantWebhookDestination
}

// OfferInstallment is one installment in the projected schedule.
type OfferInstallment struct {
	// InstallmentOrder is the 1-based position in the schedule.
	InstallmentOrder int

	// DueAt is when the installment becomes payable.
	DueAt time.Time

	// TotalAmountMinor is the full installment amount in minor currency units.
	TotalAmountMinor uint64
}

// Offer projects the checkout Decision economics for a bound customer.
//
// Risk reason codes are intentionally absent: a denied outcome reports that
// financing is unavailable without explaining why.
type Offer struct {
	// Outcome reports whether BNPL can finance this checkout.
	Outcome OfferOutcome

	// AvailableCreditMinor is the evaluation-time available credit in minor units.
	AvailableCreditMinor uint64

	// RequestedAmountMinor is the merchant-requested amount in minor units.
	RequestedAmountMinor uint64

	// ApprovedFinancedAmountMinor is the financed amount BNPL approved in minor units.
	ApprovedFinancedAmountMinor uint64

	// RequiredDownPaymentAmountMinor is the amount the customer must fund up front.
	RequiredDownPaymentAmountMinor uint64

	// Currency is the currency of all monetary fields.
	Currency model.Currency

	// OfferExpiresAt is when the offer stops being acceptable.
	OfferExpiresAt *time.Time

	// InstallmentCount is the number of installments in the approved schedule.
	InstallmentCount int

	// PayableNowAmountMinor is the amount payable immediately in minor units.
	PayableNowAmountMinor uint64

	// TotalPayableAmountMinor is the total payable amount in minor units.
	TotalPayableAmountMinor uint64

	// Installments lists the projected schedule.
	Installments []OfferInstallment

	// CheckoutAction tells the client how an approved offer is confirmed.
	CheckoutAction *decisionuc.CheckoutAction
}

func (r CheckoutSessionRequest) validate() error {
	if r.MerchantID.IsZero() {
		return merchantIDRequiredError()
	}

	if r.OnlineStoreID.IsZero() {
		return onlineStoreIDRequiredError()
	}

	if r.MerchantOrderID == "" {
		return merchantOrderIDRequiredError()
	}

	if len(r.MerchantOrderID) > model.MaxCheckoutSessionMerchantOrderIDLength {
		return merchantOrderIDTooLongError()
	}

	if len(r.CustomerReference) > model.MaxCheckoutSessionCustomerReferenceLength {
		return customerReferenceTooLongError()
	}

	if err := returndestination.ValidateDestinationName(r.ReturnDestination); err != nil {
		return mapDestinationNameError(err, "return_destination")
	}

	if len(r.MerchantState) > model.MaxCheckoutSessionMerchantStateLength {
		return merchantStateTooLongError()
	}

	if r.RequestedAmountMinor == 0 {
		return requestedAmountRequiredError()
	}

	if r.RequestedAmountMinor > math.MaxInt64 {
		return requestedAmountOutOfRangeError()
	}

	if !r.Currency.IsValid() {
		return currencyInvalidError()
	}

	return nil
}

func (i CreatePartnerInput) validate() error {
	if err := i.CheckoutSessionRequest.validate(); err != nil {
		return err
	}

	if i.CustomerPhone == "" {
		return customerPhoneRequiredError()
	}

	if len(i.CustomerPhone) > model.MaxCustomerPhoneLength {
		return customerPhoneInvalidError()
	}

	// Jurisdiction-specific syntax and canonicalization run after
	// MerchantProfile.Jurisdiction is known (createOrReplaySessionTx).

	if i.TenantID.IsZero() {
		return cerr.InternalError(fmt.Errorf("create partner: trusted tenant id is required"))
	}

	if i.ContextID.IsZero() {
		return cerr.InternalError(fmt.Errorf("create partner: trusted context id is required"))
	}

	return nil
}

func (i CreateInternalInput) validate() error {
	if err := i.CheckoutSessionRequest.validate(); err != nil {
		return err
	}

	if i.IdentitySubjectID.IsZero() {
		return identitySubjectIDRequiredError()
	}

	return nil
}

func (i StartInput) validate() error {
	if i.CheckoutSessionID.IsZero() {
		return checkoutSessionIDRequiredError()
	}

	if i.IdentitySubjectID.IsZero() {
		return identitySubjectIDRequiredError()
	}

	return nil
}

func (i ConfirmInput) validate() error {
	if i.CheckoutSessionID.IsZero() {
		return checkoutSessionIDRequiredError()
	}

	if i.IdentitySubjectID.IsZero() {
		return identitySubjectIDRequiredError()
	}

	if i.TenantID.IsZero() {
		return validationError("REQUIRED", "tenant_id", "error.tenant_id_required")
	}

	if i.ContextID.IsZero() {
		return validationError("REQUIRED", "context_id", "error.context_id_required")
	}

	if i.EvaluationGeneration < 1 {
		return validationError(
			"REQUIRED",
			"evaluation_generation",
			"error.evaluation_generation_required",
		)
	}

	return nil
}

// VerificationTypeSMSOTP identifies phone verification by an SMS one-time code.
const VerificationTypeSMSOTP = "sms_otp"

// GetExternalInput carries the hosted page read of an external checkout.
type GetExternalInput struct {
	// Token is the public checkout session token from the hosted URL.
	Token model.CheckoutSessionToken
}

func (i GetExternalInput) validate() error {
	return validateCheckoutSessionToken(i.Token)
}

// ExternalVerification describes how the customer proves the checkout phone.
type ExternalVerification struct {
	// Type is the verification method, currently always VerificationTypeSMSOTP.
	Type string

	// Destination is the masked phone the code is delivered to.
	Destination string
}

// GetExternalOutput is the read-only hosted checkout projection.
//
// For a prepared open checkout it includes the safe purchase offer the customer
// must see before Continue → START. It never includes the full phone, internal
// identifiers, customer ids, available_credit_minor, or customer_reference.
type GetExternalOutput struct {
	// Token is the public checkout identifier.
	Token model.CheckoutSessionToken

	// Status is the effective status. Open sessions past expires_at are
	// projected as expired without a write.
	Status model.CheckoutSessionStatus

	// MerchantDisplayName is the merchant's public trade name.
	MerchantDisplayName string

	// AmountMinor is the requested checkout amount in minor units.
	AmountMinor uint64

	// Currency is the checkout currency.
	Currency model.Currency

	// EvaluationGeneration is the offer version Hosted START/Confirm must echo.
	EvaluationGeneration int

	// Offer is the persisted financing projection when a Decision is linked.
	// Nil when preparation has not produced a projectable offer yet.
	Offer *Offer

	// Verification describes the phone possession proof step (mask only).
	Verification ExternalVerification

	// ExpiresAt is when the open handoff stops being usable.
	ExpiresAt time.Time
}

// ReturnExternalInput carries the hosted browser-return navigation request.
type ReturnExternalInput struct {
	// Token is the public checkout session token from the hosted URL.
	Token model.CheckoutSessionToken
}

func (i ReturnExternalInput) validate() error {
	return validateCheckoutSessionToken(i.Token)
}

// ReturnExternalOutput carries the server-built merchant return URL.
//
// RedirectURL is constructed only from the persisted ReturnURL and safe query
// parameters (public token, optional merchant state). Callers must never treat
// browser arrival at this URL as payment or fulfillment authority.
type ReturnExternalOutput struct {
	// RedirectURL is the absolute merchant return destination.
	RedirectURL string
}

// StartExternalInput carries the hosted external OTP request.
//
// EvaluationGeneration must be the exact value returned by Hosted GET so the
// OTP authorizes only the offer terms the customer saw.
type StartExternalInput struct {
	// Token is the public checkout session token from the hosted URL.
	Token model.CheckoutSessionToken

	// EvaluationGeneration is the offer version presented by Hosted GET.
	EvaluationGeneration int
}

func (i StartExternalInput) validate() error {
	if err := validateCheckoutSessionToken(i.Token); err != nil {
		return err
	}

	if i.EvaluationGeneration < 1 {
		return validationError(
			"REQUIRED",
			"evaluation_generation",
			"error.evaluation_generation_required",
		)
	}

	return nil
}

// StartExternalOutput returns OTP delivery state for a prepared offer.
//
// The full offer is returned by Hosted GET, not by successful START.
type StartExternalOutput struct {
	// Session is the checkout session after OTP issuance (or idempotent replay).
	Session model.CheckoutSession

	// EvaluationGeneration is the offer version this OTP challenge authorizes.
	EvaluationGeneration int

	// OTPSent is true when this call issued a new OTP and Notification accepted
	// its submission. It is false
	// for idempotent replays that found an active/in-flight challenge.
	OTPSent bool

	// OTPExpiresAt is when the active one-time code stops being accepted. It is
	// nil when no code is active.
	OTPExpiresAt *time.Time
}

// ConfirmExternalInput carries the hosted external Confirm & Pay request.
type ConfirmExternalInput struct {
	// Token is the public checkout session token from the hosted URL.
	Token model.CheckoutSessionToken

	// OTP is the six-digit code the customer received by SMS.
	OTP string

	// EvaluationGeneration is the offer version presented to the customer.
	EvaluationGeneration int
}

func (i ConfirmExternalInput) validate() error {
	if err := validateCheckoutSessionToken(i.Token); err != nil {
		return err
	}

	if i.OTP == "" {
		return checkoutOTPRequiredError()
	}

	if !checkoutotp.IsWellFormed(i.OTP) {
		return checkoutOTPInvalidError(nil)
	}

	if i.EvaluationGeneration < 1 {
		return validationError(
			"REQUIRED",
			"evaluation_generation",
			"error.evaluation_generation_required",
		)
	}

	return nil
}

func validateCheckoutSessionToken(token model.CheckoutSessionToken) error {
	if err := token.Validate(); err != nil {
		return checkoutSessionTokenInvalidError(err)
	}

	return nil
}
