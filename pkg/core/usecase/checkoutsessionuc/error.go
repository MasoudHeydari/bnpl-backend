package checkoutsessionuc

import (
	"errors"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// validationError builds a single-field public validation error. Every checkout
// session input rejection has the same shape, so the field name and violation
// code are the only things that vary.
func validationError(code string, field string, messageKey cerr.MessageKey) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		messageKey,
		nil,
		cerr.Details{
			Fields: []string{field},
			Violations: []cerr.Violation{
				{
					Code:       code,
					MessageKey: messageKey,
					Fields:     []string{field},
				},
			},
		},
	)
}

func merchantIDRequiredError() error {
	return validationError("REQUIRED", "merchant_id", "error.merchant_id_required")
}

func externalCustomerIDRequiredError() error {
	return validationError("REQUIRED", "identity_subject_id", "error.identity_subject_id_required")
}

func checkoutSessionIDRequiredError() error {
	return validationError("REQUIRED", "checkout_session_id", "error.checkout_session_id_required")
}

func checkoutSessionTokenInvalidError(cause error) error {
	_ = cause

	return validationError("INVALID", "checkout_session_id", "error.checkout_session_id_invalid")
}

func onlineStoreIDRequiredError() error {
	return validationError("REQUIRED", "online_store_id", "error.online_store_id_required")
}

func merchantOrderIDRequiredError() error {
	return validationError("REQUIRED", "merchant_order_id", "error.merchant_order_id_required")
}

func merchantOrderIDTooLongError() error {
	return validationError("MAX_LENGTH", "merchant_order_id", "error.merchant_order_id_too_long")
}

func customerReferenceTooLongError() error {
	return validationError("MAX_LENGTH", "customer_reference", "error.customer_reference_too_long")
}

func customerPhoneRequiredError() error {
	return validationError("REQUIRED", "customer_phone", "error.customer_phone_required")
}

func customerPhoneInvalidError() error {
	return validationError("INVALID", "customer_phone", "error.customer_phone_invalid")
}

func returnDestinationRequiredError() error {
	return validationError("REQUIRED", "return_destination", "error.return_destination_required")
}

func returnDestinationTooLongError() error {
	return validationError("MAX_LENGTH", "return_destination", "error.return_destination_too_long")
}

func returnDestinationInvalidError() error {
	return validationError("INVALID", "return_destination", "error.return_destination_invalid")
}

func destinationNameRequiredError() error {
	return validationError("REQUIRED", "destination_name", "error.destination_name_required")
}

func destinationNameTooLongError() error {
	return validationError("MAX_LENGTH", "destination_name", "error.destination_name_too_long")
}

func destinationNameInvalidError() error {
	return validationError("INVALID", "destination_name", "error.destination_name_invalid")
}

func returnURLRequiredError() error {
	return validationError("REQUIRED", "return_url", "error.return_url_required")
}

func returnURLTooLongError() error {
	return validationError("MAX_LENGTH", "return_url", "error.return_url_too_long")
}

func returnURLInvalidError() error {
	return validationError("INVALID", "return_url", "error.return_url_invalid")
}

func returnURLMustUseHTTPSError() error {
	return validationError("INVALID", "return_url", "error.return_url_must_use_https")
}

func merchantStateTooLongError() error {
	return validationError("MAX_LENGTH", "merchant_state", "error.merchant_state_too_long")
}

func requestedAmountRequiredError() error {
	return validationError(
		"REQUIRED",
		"requested_amount_minor",
		"error.requested_amount_must_be_greater_than_zero",
	)
}

func requestedAmountOutOfRangeError() error {
	return validationError(
		"OUT_OF_RANGE",
		"requested_amount_minor",
		"error.requested_amount_exceeds_supported_range",
	)
}

func currencyInvalidError() error {
	return validationError("INVALID", "currency", "error.currency_invalid")
}

// returnDestinationNotRegisteredError rejects a destination name the merchant
// never registered. Accepting an arbitrary URL instead would turn the hosted
// handoff into an open redirect, so an unknown name is always a hard failure.
func returnDestinationNotRegisteredError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.merchant_return_destination_is_not_registered",
		cause,
		cerr.Details{
			Fields: []string{"return_destination"},
			Violations: []cerr.Violation{
				{
					Code:       "NOT_REGISTERED",
					MessageKey: "error.merchant_return_destination_is_not_registered",
					Fields:     []string{"return_destination"},
				},
			},
		},
	)
}

// onlineStoreNotOwnedError rejects a storefront that resolves to a different
// merchant than the authenticated one.
func onlineStoreNotOwnedError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeForbidden,
		"error.online_store_is_not_owned_by_the_authenticated_merchant",
		cause,
		cerr.Details{
			Fields: []string{"online_store_id"},
			Violations: []cerr.Violation{
				{
					Code:       "FORBIDDEN",
					MessageKey: "error.online_store_is_not_owned_by_the_authenticated_merchant",
					Fields:     []string{"online_store_id"},
				},
			},
		},
	)
}

// merchantNotActiveError rejects checkout creation for a merchant that is not
// currently allowed to offer BNPL.
func merchantNotActiveError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeForbidden,
		"error.merchant_is_not_active_for_bnpl",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutSessionNotFoundError is returned for both missing sessions and
// sessions owned by someone else, so a caller cannot probe for foreign
// checkouts.
func checkoutSessionNotFoundError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeResourceNotFound,
		"error.checkout_session_not_found",
		cause,
		cerr.Details{
			Fields:     []string{"checkout_session_id"},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutCustomerIdentityConflictError blocks continuation when the checkout
// is already bound to a different customer than the authenticated or resolved
// subject. This is an internal identity invariant, not a merchant customer
// binding.
func checkoutCustomerIdentityConflictError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeForbidden,
		"error.checkout_customer_identity_conflict",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutSessionExpiredError reports an elapsed handoff lifetime.
func checkoutSessionExpiredError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_session_expired",
		nil,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutSessionNotContinuableError reports a session state that no longer
// accepts customer evaluation.
func checkoutSessionNotContinuableError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_session_cannot_continue_in_its_current_state",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutSessionNotCancelableError reports a session state that cannot be
// canceled.
func checkoutSessionNotCancelableError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_session_cannot_be_canceled_in_its_current_state",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutPaymentInProgressError refuses to close a checkout whose payment
// outcome is still owned by Financial Core. Canceling here could race a
// successful payment.
func checkoutPaymentInProgressError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_payment_is_in_progress_and_must_be_resolved_first",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// merchantOrderConflictError reports a reused merchant order reference carrying
// different commercial terms. The stored session is never mutated.
func merchantOrderConflictError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeIdempotencyConflict,
		"error.merchant_order_id_conflicts_with_an_existing_checkout_session",
		cause,
		cerr.Details{
			Fields: []string{"merchant_order_id"},
			Violations: []cerr.Violation{
				{
					Code:       "IDEMPOTENCY_CONFLICT",
					MessageKey: "error.merchant_order_id_conflicts_with_an_existing_checkout_session",
					Fields:     []string{"merchant_order_id"},
				},
			},
		},
	)
}

// checkoutSessionCreateUnresolvedConflictError is returned when a concurrent
// create raced and the use case could not reconcile to a stable session.
func checkoutSessionCreateUnresolvedConflictError() error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_session_could_not_be_created_due_to_a_concurrent_conflict",
		nil,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

func checkoutOfferNotReadyError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_offer_is_not_ready",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

func checkoutOfferReconfirmRequiredError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_offer_reconfirm_required",
		cause,
		cerr.Details{
			Fields:     []string{"evaluation_generation"},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutOfferUnavailableError is returned when a refreshed evaluation denies
// financing. The CheckoutSession is already transitioned to unavailable; this
// is not a reconfirm_required result and must not carry an unusable offer.
func checkoutOfferUnavailableError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_offer_unavailable",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// isDeterministicOfferNoLongerHonorable reports whether PrepareCheckoutContract
// failed with a known deterministic pre-contract rejection that proves no
// Contract was committed for the current Decision (insufficient credit,
// unusable credit account, or Decision/allocation no longer live).
func isDeterministicOfferNoLongerHonorable(err error) bool {
	var appErr *cerr.Error
	if !errors.As(err, &appErr) {
		return false
	}

	switch appErr.MessageKey {
	case "error.insufficient_credit_for_contract",
		"error.credit_account_is_not_usable_for_new_contract",
		"error.checkout_decision_allocation_expired":
		return true
	}

	for _, violation := range appErr.Details.Violations {
		switch violation.Code {
		case "INSUFFICIENT_CREDIT",
			"CREDIT_ACCOUNT_NOT_USABLE",
			"CHECKOUT_DECISION_ALLOCATION_EXPIRED":
			return true
		}
	}

	return false
}

func webhookSigningSecretRequiredError() error {
	return validationError("REQUIRED", "signing_secret", "error.webhook_signing_secret_required")
}

func webhookSigningSecretTooShortError() error {
	return validationError("MIN_LENGTH", "signing_secret", "error.webhook_signing_secret_too_short")
}

func webhookSigningSecretTooLongError() error {
	return validationError("MAX_LENGTH", "signing_secret", "error.webhook_signing_secret_too_long")
}

// checkoutOTPRequiredError rejects a confirm that carries no one-time code.
func checkoutOTPRequiredError() error {
	return validationError("REQUIRED", "otp", "error.checkout_otp_required")
}

// checkoutOTPInvalidError rejects a malformed or wrong one-time code. The cause
// is kept for logs only and never distinguishes malformed from wrong input.
func checkoutOTPInvalidError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.checkout_otp_invalid",
		cause,
		cerr.Details{
			Fields: []string{"otp"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.checkout_otp_invalid",
					Fields:     []string{"otp"},
				},
			},
		},
	)
}

// checkoutOTPExpiredError reports an issued one-time code past its lifetime.
// The customer must restart the checkout verification to receive a new code.
func checkoutOTPExpiredError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_otp_expired",
		cause,
		cerr.Details{
			Fields:     []string{"otp"},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutOTPLockedError reports that the wrong-code budget of the issued
// one-time code is exhausted. Only a fresh code can unlock verification.
func checkoutOTPLockedError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeForbidden,
		"error.checkout_otp_locked",
		cause,
		cerr.Details{
			Fields:     []string{"otp"},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutOTPNotRequestedError reports a confirm that arrived before any
// one-time code was issued for the checkout.
func checkoutOTPNotRequestedError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.checkout_otp_not_requested",
		cause,
		cerr.Details{
			Fields:     []string{"otp"},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutOTPSendCooldownError reports a new OTP requested before the policy
// send cooldown has elapsed.
func checkoutOTPSendCooldownError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeTooManyRequests,
		"error.checkout_otp_send_cooldown",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutOTPSendLimitError reports that the checkout has exhausted its OTP
// send budget under the security policy.
func checkoutOTPSendLimitError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeTooManyRequests,
		"error.checkout_otp_send_limit_reached",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutOTPRateLimitedError reports that a cross-checkout OTP send budget
// (phone or merchant) was exceeded. The public message is intentionally
// generic so clients cannot learn which bucket rejected the request.
func checkoutOTPRateLimitedError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeTooManyRequests,
		"error.checkout_otp_rate_limited",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// checkoutCustomerNotEligibleError reports a verified phone the Identity
// Service does not know, so BNPL cannot finance this customer.
func checkoutCustomerNotEligibleError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeForbidden,
		"error.checkout_customer_is_not_eligible",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}
