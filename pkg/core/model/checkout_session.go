package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Bounds for merchant-supplied checkout session values. They exist so an
// integration cannot store unbounded opaque data behind the hosted checkout
// boundary.
const (
	// MaxCheckoutSessionMerchantOrderIDLength bounds the merchant-owned order reference.
	MaxCheckoutSessionMerchantOrderIDLength = 256

	// MaxCheckoutSessionCustomerReferenceLength bounds the merchant-owned customer reference.
	MaxCheckoutSessionCustomerReferenceLength = 256

	// MaxCheckoutSessionMerchantStateLength bounds the opaque merchant correlation value.
	MaxCheckoutSessionMerchantStateLength = 256

	// MaxCustomerPhoneLength bounds the raw merchant-asserted customer phone
	// string before jurisdiction-specific canonicalization.
	MaxCustomerPhoneLength = 32
)

// CheckoutSessionID is the internal BNPL primary key for a merchant checkout
// session (checkout_sessions.id).
//
// It is INTERNAL only: it is used for foreign keys, locking, and logs, and is
// never exposed to merchants or browsers. Public boundaries address a checkout
// by its CheckoutSessionToken instead.
type CheckoutSessionID uuid.UUID

// ParseCheckoutSessionID parses a CheckoutSessionID from its canonical UUID string form.
func ParseCheckoutSessionID(value string) (CheckoutSessionID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return CheckoutSessionID{}, fmt.Errorf("parse checkout session id: %w", err)
	}

	return CheckoutSessionID(id), nil
}

// UUID returns the underlying uuid.UUID value for boundary conversions.
func (id CheckoutSessionID) UUID() uuid.UUID { return uuid.UUID(id) }

// String returns the canonical UUID string representation.
func (id CheckoutSessionID) String() string { return uuid.UUID(id).String() }

// IsZero reports whether id is the zero (nil) UUID.
func (id CheckoutSessionID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

// MarshalText implements encoding.TextMarshaler.
func (id CheckoutSessionID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

// UnmarshalText implements encoding.TextUnmarshaler.
func (id *CheckoutSessionID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = CheckoutSessionID(u)

	return nil
}

// Scan implements sql.Scanner.
func (id *CheckoutSessionID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = CheckoutSessionID(u)

	return nil
}

// Value implements driver.Valuer.
func (id CheckoutSessionID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// CheckoutSessionStatus is the lifecycle state of a merchant checkout handoff.
//
// The status describes the checkout conversation, not payment authority.
// A failed Financial Core payment attempt does not terminate the checkout:
// the customer may retry inside the same open session after a retryable failure.
type CheckoutSessionStatus string

const (
	// OpenCheckoutSessionStatus indicates the handoff is usable and the customer may continue checkout.
	OpenCheckoutSessionStatus CheckoutSessionStatus = "open"

	// ConfirmingCheckoutSessionStatus indicates the customer accepted the presented
	// offer and Confirm owns the session while contract/payment initialization runs.
	// Cancel and expiry must not close a confirming session.
	ConfirmingCheckoutSessionStatus CheckoutSessionStatus = "confirming"

	// ProcessingCheckoutSessionStatus indicates an authoritative external payment outcome is pending.
	ProcessingCheckoutSessionStatus CheckoutSessionStatus = "processing"

	// CompletedCheckoutSessionStatus indicates the checkout produced an activated contract.
	CompletedCheckoutSessionStatus CheckoutSessionStatus = "completed"

	// CanceledCheckoutSessionStatus indicates the merchant or customer abandoned the checkout.
	CanceledCheckoutSessionStatus CheckoutSessionStatus = "canceled"

	// ExpiredCheckoutSessionStatus indicates the handoff lifetime elapsed before completion.
	ExpiredCheckoutSessionStatus CheckoutSessionStatus = "expired"

	// UnavailableCheckoutSessionStatus indicates BNPL financing could not be offered
	// for this checkout. Unavailable is terminal for the session; a later eligible
	// purchase requires a new checkout attempt.
	UnavailableCheckoutSessionStatus CheckoutSessionStatus = "unavailable"
)

// CheckoutSessionOrigin records which create path produced a checkout session.
//
// It decides how customer identity is established. External partner sessions
// resolve the Merchant-asserted verified phone through Identity during Partner
// Create, then require OTP later as possession proof and authorization of the
// exact offer generation. Internal sessions are created already bound to a
// trusted customer.
type CheckoutSessionOrigin string

const (
	// ExternalPartnerCheckoutSessionOrigin indicates the session was created by
	// an external merchant through the partner API.
	ExternalPartnerCheckoutSessionOrigin CheckoutSessionOrigin = "external_partner"

	// InternalCheckoutSessionOrigin indicates the session was created by a
	// trusted first-party caller on behalf of a known customer.
	InternalCheckoutSessionOrigin CheckoutSessionOrigin = "internal"
)

// IsValid reports whether the origin is a known canonical value.
func (o CheckoutSessionOrigin) IsValid() bool {
	switch o {
	case ExternalPartnerCheckoutSessionOrigin, InternalCheckoutSessionOrigin:
		return true

	default:
		return false
	}
}

// CheckoutCancellationReason records who abandoned a checkout session.
//
// Merchant-facing status remains "canceled". The reason exists so integration
// events can distinguish customer vs merchant abandonment without inventing
// parallel status values.
type CheckoutCancellationReason string

const (
	// CustomerCanceledCheckoutReason indicates the authenticated customer abandoned checkout.
	CustomerCanceledCheckoutReason CheckoutCancellationReason = "customer_canceled"

	// MerchantCanceledCheckoutReason indicates the owning merchant canceled the checkout.
	MerchantCanceledCheckoutReason CheckoutCancellationReason = "merchant_canceled"
)

// IsValid reports whether the cancellation reason is a known canonical value.
func (r CheckoutCancellationReason) IsValid() bool {
	switch r {
	case CustomerCanceledCheckoutReason, MerchantCanceledCheckoutReason:
		return true

	default:
		return false
	}
}

var checkoutSessionStatusTransitions = newStatusTransitionTable(
	"checkout session status",
	map[CheckoutSessionStatus][]CheckoutSessionStatus{
		OpenCheckoutSessionStatus: {
			ConfirmingCheckoutSessionStatus,
			CanceledCheckoutSessionStatus,
			ExpiredCheckoutSessionStatus,
			UnavailableCheckoutSessionStatus,
		},
		// confirming owns the session through contract/payment initialization.
		// Cancel and expiry refuse it. Merchant-funding may jump to completed;
		// initial-payment advances to processing; a failed init before FC may
		// return to open so the customer can retry Confirm.
		ConfirmingCheckoutSessionStatus: {
			ProcessingCheckoutSessionStatus,
			CompletedCheckoutSessionStatus,
			OpenCheckoutSessionStatus,
		},
		// processing may only leave on an authoritative payment outcome. The
		// cancel/expire APIs refuse to act on it because the outcome is ambiguous.
		ProcessingCheckoutSessionStatus: {
			CompletedCheckoutSessionStatus,
			// Retryable payment failure returns the session to open so the
			// customer may confirm again against the same Contract. Completed
			// never regresses.
			OpenCheckoutSessionStatus,
		},
	},
	[]CheckoutSessionStatus{
		CompletedCheckoutSessionStatus,
		CanceledCheckoutSessionStatus,
		ExpiredCheckoutSessionStatus,
		UnavailableCheckoutSessionStatus,
	},
)

// IsValid reports whether the checkout session status is known by the domain.
func (s CheckoutSessionStatus) IsValid() bool {
	return checkoutSessionStatusTransitions.isValid(s)
}

// CanTransitionTo reports whether the checkout session status may change to next.
func (s CheckoutSessionStatus) CanTransitionTo(next CheckoutSessionStatus) bool {
	return checkoutSessionStatusTransitions.canTransitionTo(s, next)
}

// ValidateTransitionTo returns an error when the checkout session status may not change to next.
func (s CheckoutSessionStatus) ValidateTransitionTo(next CheckoutSessionStatus) error {
	return checkoutSessionStatusTransitions.validateTransitionTo(s, next)
}

// IsTerminal reports whether the checkout session status is terminal.
func (s CheckoutSessionStatus) IsTerminal() bool {
	return checkoutSessionStatusTransitions.isTerminal(s)
}

// AllowsCustomerContinuation reports whether a customer may still evaluate or
// confirm financing inside this checkout session.
func (s CheckoutSessionStatus) AllowsCustomerContinuation() bool {
	return s == OpenCheckoutSessionStatus
}

// BlocksCancelAndExpiry reports whether cancel APIs and the expiry job must
// refuse to close the session because Confirm or an in-flight payment owns it.
func (s CheckoutSessionStatus) BlocksCancelAndExpiry() bool {
	switch s {
	case ConfirmingCheckoutSessionStatus, ProcessingCheckoutSessionStatus:
		return true

	default:
		return false
	}
}

// IsActiveCheckoutAttempt reports whether the session still occupies the single
// non-terminal slot for its merchant order.
func (s CheckoutSessionStatus) IsActiveCheckoutAttempt() bool {
	switch s {
	case OpenCheckoutSessionStatus,
		ConfirmingCheckoutSessionStatus,
		ProcessingCheckoutSessionStatus:
		return true

	default:
		return false
	}
}

// CheckoutSession is the merchant integration boundary for one BNPL checkout.
//
// It carries merchant-supplied correlation data and the resolved return
// destination, and links the checkout to the Identity-resolved customer,
// personalized Decision, and eventual Contract. For external checkouts,
// resolving identity does not authorize the purchase: OTP later authorizes the
// exact offer generation. Merchants never receive customer credit facts from
// this record; those remain on the Hosted customer boundary.
type CheckoutSession struct {
	// ID is the internal primary key. It is never exposed to merchants or browsers.
	ID CheckoutSessionID `db:"id" json:"id"`

	// PublicToken is the unguessable public handle shared with merchants and browsers.
	PublicToken CheckoutSessionToken `db:"public_token" json:"public_token"`

	// Origin records which create path produced the session.
	Origin CheckoutSessionOrigin `db:"origin" json:"origin"`

	// MerchantProfileID is the owning merchant BNPL profile primary key.
	MerchantProfileID MerchantProfileID `db:"merchant_profile_id" json:"merchant_profile_id"`

	// MerchantBranchProfileID is the online selling point resolved from OnlineStoreID.
	MerchantBranchProfileID MerchantBranchProfileID `db:"merchant_branch_profile_id" json:"merchant_branch_profile_id"`

	// OnlineStoreID is the external storefront identity supplied by the merchant.
	OnlineStoreID OnlineStoreID `db:"online_store_id" json:"online_store_id"`

	// MerchantOrderID is the merchant-owned business order reference. Multiple
	// checkout attempts may share one order after prior attempts terminalize.
	MerchantOrderID string `db:"merchant_order_id" json:"merchant_order_id"`

	// IdempotencyKey uniquely identifies one create request / checkout attempt
	// for the merchant. Replays with the same key and payload return this session.
	IdempotencyKey string `db:"idempotency_key" json:"idempotency_key"`

	// CustomerReference is an optional merchant-owned correlation handle echoed
	// for the merchant's own bookkeeping. It is NOT an identity mapping: BNPL
	// never resolves a customer from it.
	CustomerReference string `db:"customer_reference" json:"customer_reference"`

	// CustomerPhoneCiphertext is the envelope-encrypted canonical E.164 phone the
	// merchant supplied for OTP verification. Never serialized.
	CustomerPhoneCiphertext *string `db:"customer_phone_ciphertext" json:"-"`

	// CustomerPhoneKeyID identifies the key that encrypted CustomerPhoneCiphertext.
	CustomerPhoneKeyID *string `db:"customer_phone_key_id" json:"-"`

	// OTPIssueID is the opaque identity of the currently issued OTP challenge.
	// Every IssueOTP call creates a new identity; MarkOTPNotificationAccepted and definite-rejection
	// failure cleanup are compare-and-swap scoped to it. Never serialized.
	OTPIssueID *OTPIssueID `db:"otp_issue_id" json:"-"`

	// OTPVerifier is the keyed hash of the currently issued OTP. The plaintext
	// code is never stored. Never serialized.
	OTPVerifier *string `db:"otp_verifier" json:"-"`

	// OTPExpiresAt is when the currently issued OTP stops being accepted.
	OTPExpiresAt *time.Time `db:"otp_expires_at" json:"-"`

	// OTPNotificationAcceptedAt is when Notification durably accepted submission of the current OTP issuance.
	OTPNotificationAcceptedAt *time.Time `db:"otp_notification_accepted_at" json:"-"`

	// OTPVerifiedAt is set once the customer proved possession of the phone.
	OTPVerifiedAt *time.Time `db:"otp_verified_at" json:"-"`

	// OTPAttemptCount counts failed verification attempts for the current OTP.
	OTPAttemptCount int `db:"otp_attempt_count" json:"-"`

	// OTPNotificationRequestID is the Notification Service request identifier for the accepted OTP submission.
	OTPNotificationRequestID *string `db:"otp_notification_request_id" json:"-"`

	// OTPSendCount is the number of one-time codes issued for this checkout. It
	// is never reset and enforces the per-checkout send cap.
	OTPSendCount int `db:"otp_send_count" json:"-"`

	// OTPEvaluationGeneration is the EvaluationGeneration the issued or verified
	// code is bound to. It is kept after verification so Confirm can prove the
	// verification belongs to the offer being confirmed.
	OTPEvaluationGeneration *int `db:"otp_evaluation_generation" json:"-"`

	// OfferPreparationMode is snapshotted from MerchantBranchProfile at create time.
	OfferPreparationMode OfferPreparationMode `db:"offer_preparation_mode" json:"-"`

	// CustomerID is the internal customer profile primary key, set once BNPL trusts the customer identity.
	CustomerID *CustomerID `db:"customer_id" json:"customer_id,omitempty"`

	// ReturnDestination is the symbolic name of a pre-registered merchant return destination.
	ReturnDestination string `db:"return_destination" json:"return_destination"`

	// ReturnURL is the absolute URL resolved from ReturnDestination at creation time.
	ReturnURL string `db:"return_url" json:"return_url"`

	// MerchantState is opaque merchant correlation data echoed back on browser return.
	MerchantState string `db:"merchant_state" json:"merchant_state"`

	// RequestedAmountMinor is the checkout amount requested by the merchant in minor units.
	RequestedAmountMinor uint64 `db:"requested_amount_minor" json:"requested_amount_minor"`

	// Currency is the currency of RequestedAmountMinor.
	Currency Currency `db:"currency" json:"currency"`

	// Status is the current checkout session lifecycle state.
	Status CheckoutSessionStatus `db:"status" json:"status"`

	// DecisionID links the personalized checkout Decision created for the bound customer.
	DecisionID *DecisionID `db:"decision_id" json:"decision_id,omitempty"`

	// ContractID links the BNPL contract created from the accepted offer.
	ContractID *ContractID `db:"contract_id" json:"contract_id,omitempty"`

	// CancellationReason records who abandoned the checkout when Status is canceled.
	// Merchant-facing status stays "canceled"; integration events may echo the reason.
	CancellationReason *CheckoutCancellationReason `db:"cancellation_reason" json:"cancellation_reason,omitempty"`

	// EvaluationGeneration increments when a new Decision must be created for this
	// session (for example after the previous offer expired). It is part of the
	// Decision idempotency key so prior Decisions remain immutable history.
	EvaluationGeneration int `db:"evaluation_generation" json:"evaluation_generation"`

	// ExpiresAt is when an open handoff stops being usable.
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`

	// CompletedAt is set when Status becomes completed.
	CompletedAt *time.Time `db:"completed_at" json:"completed_at,omitempty"`

	// CanceledAt is set when Status becomes canceled.
	CanceledAt *time.Time `db:"canceled_at" json:"canceled_at,omitempty"`

	// CreatedAt is when the session row was created.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the session row was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// IsExpiredAt reports whether the handoff lifetime has elapsed at now.
func (s CheckoutSession) IsExpiredAt(now time.Time) bool {
	return !now.Before(s.ExpiresAt)
}

// EffectiveOfferPreparationMode returns the snapshotted preparation mode.
func (s CheckoutSession) EffectiveOfferPreparationMode() OfferPreparationMode {
	if s.OfferPreparationMode.IsValid() {
		return s.OfferPreparationMode
	}

	return DefaultOfferPreparationMode
}

// MatchesCreateRequest reports whether an idempotent replay carries the same
// commercial terms as the stored session. A mismatch is an idempotency conflict
// and must never mutate the stored session.
func (s CheckoutSession) MatchesCreateRequest(
	onlineStoreID OnlineStoreID,
	merchantOrderID string,
	customerReference string,
	returnDestination string,
	merchantState string,
	requestedAmountMinor uint64,
	currency Currency,
) bool {
	return s.OnlineStoreID == onlineStoreID &&
		s.MerchantOrderID == merchantOrderID &&
		s.CustomerReference == customerReference &&
		s.ReturnDestination == returnDestination &&
		s.MerchantState == merchantState &&
		s.RequestedAmountMinor == requestedAmountMinor &&
		s.Currency == currency
}
