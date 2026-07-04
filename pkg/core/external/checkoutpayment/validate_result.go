package checkoutpayment

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultMaxFinancialIdentifierLength is the maximum length for FC identifiers
	// accepted on CreateCheckoutPayment responses used by BNPL.
	DefaultMaxFinancialIdentifierLength = 256

	// DefaultMaxPaymentRedirectURLLength is the maximum length for a customer
	// payment redirect URL returned by Financial Core.
	DefaultMaxPaymentRedirectURLLength = 2048
)

var (
	// ErrIncompletePaymentIdentifiers reports missing or oversized FC intent/session ids.
	ErrIncompletePaymentIdentifiers = errors.New("financial core returned incomplete or oversized payment identifiers")

	// ErrUnsupportedPaymentStatus reports a CreateCheckoutPayment status this
	// BNPL operation cannot treat as an actionable customer payment session.
	ErrUnsupportedPaymentStatus = errors.New("financial core returned unsupported payment status for customer action")

	// ErrAmountCurrencyMismatch reports a response amount/currency that does not
	// match the requested CreateCheckoutPayment economics.
	ErrAmountCurrencyMismatch = errors.New("financial core returned payment amount or currency mismatch")

	// ErrMissingPaymentExpiry reports a missing FC PaymentSession expiration.
	ErrMissingPaymentExpiry = errors.New("financial core returned payment expiry is missing")

	// ErrPaymentExpiryNotUsable reports an FC expiration that is not strictly
	// after validation time (or outside an optional upper bound).
	ErrPaymentExpiryNotUsable = errors.New("financial core returned payment expiry is not usable")

	// ErrInvalidPaymentRedirectURL reports a missing or unsafe payment link.
	ErrInvalidPaymentRedirectURL = errors.New("financial core returned invalid payment redirect url")
)

// RequiresActionCreateResultCheck carries expected economics and time bounds for
// validating a RequiresAction CreateCheckoutPayment response before BNPL persists
// an actionable submitted PaymentAttempt.
type RequiresActionCreateResultCheck struct {
	ExpectedAmountMinor int64
	ExpectedCurrency    string
	Now                 time.Time

	// LatestAllowedExpiresAt is retained for callers that still enforce a hard
	// upper bound. Soft FC-vs-requested expiry deviations should be logged as
	// warnings by the use case rather than passed here.
	LatestAllowedExpiresAt *time.Time

	MaxIdentifierLength int
	MaxPaymentURLLength int
}

// ActionablePaymentSession is the minimum session data BNPL requires before
// exposing a customer-action PaymentAttempt as submitted.
type ActionablePaymentSession struct {
	PaymentIntentID  string
	PaymentSessionID string
	PaymentURL       string
	ExpiresAt        time.Time
}

// ValidateActionablePaymentSession validates merged session facts for an
// actionable submitted transition (create response or reconciliation pending).
func ValidateActionablePaymentSession(
	session ActionablePaymentSession,
	now time.Time,
	maxIdentifierLength int,
	maxPaymentURLLength int,
) error {
	maxID := maxIdentifierLength
	if maxID <= 0 {
		maxID = DefaultMaxFinancialIdentifierLength
	}

	maxURL := maxPaymentURLLength
	if maxURL <= 0 {
		maxURL = DefaultMaxPaymentRedirectURLLength
	}

	intentID := strings.TrimSpace(session.PaymentIntentID)
	sessionID := strings.TrimSpace(session.PaymentSessionID)
	if intentID == "" || len(intentID) > maxID ||
		sessionID == "" || len(sessionID) > maxID {
		return ErrIncompletePaymentIdentifiers
	}

	if session.ExpiresAt.IsZero() {
		return ErrMissingPaymentExpiry
	}

	if !session.ExpiresAt.UTC().After(now.UTC()) {
		return fmt.Errorf("%w: expires_at must be strictly after validation time", ErrPaymentExpiryNotUsable)
	}

	if err := ValidatePaymentRedirectURL(session.PaymentURL, maxURL); err != nil {
		return err
	}

	return nil
}

// ValidatePaymentRedirectURL enforces absolute HTTPS payment links without userinfo.
func ValidatePaymentRedirectURL(raw string, maxLength int) error {
	if maxLength <= 0 {
		maxLength = DefaultMaxPaymentRedirectURLLength
	}

	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxLength {
		return fmt.Errorf("%w: payment url is missing or too long", ErrInvalidPaymentRedirectURL)
	}

	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" || parsed.User != nil {
		return fmt.Errorf("%w: payment url must be an absolute https url without user information", ErrInvalidPaymentRedirectURL)
	}

	return nil
}

// ValidateRequiresActionCreateResult validates that a CreateCheckoutPayment
// response is a usable customer-action session for the requested economics.
//
// A validation failure means BNPL must not MarkSubmitted with actionable session
// data. It does not prove Financial Core failed to commit the operation.
func ValidateRequiresActionCreateResult(
	result CreateCheckoutPaymentResult,
	check RequiresActionCreateResultCheck,
) error {
	maxID := check.MaxIdentifierLength
	if maxID <= 0 {
		maxID = DefaultMaxFinancialIdentifierLength
	}

	maxURL := check.MaxPaymentURLLength
	if maxURL <= 0 {
		maxURL = DefaultMaxPaymentRedirectURLLength
	}

	paymentIntentID := strings.TrimSpace(result.PaymentIntentID)
	paymentSessionID := strings.TrimSpace(result.PaymentSessionID)
	if paymentIntentID == "" || len(paymentIntentID) > maxID ||
		paymentSessionID == "" || len(paymentSessionID) > maxID {
		return ErrIncompletePaymentIdentifiers
	}

	if result.Status != RequiresActionPaymentStatus {
		return fmt.Errorf("%w: %q", ErrUnsupportedPaymentStatus, result.Status)
	}

	if result.AmountMinor != check.ExpectedAmountMinor ||
		strings.TrimSpace(result.Currency) != strings.TrimSpace(check.ExpectedCurrency) {
		return ErrAmountCurrencyMismatch
	}

	if result.ExpiresAt.IsZero() {
		return ErrMissingPaymentExpiry
	}

	now := check.Now.UTC()
	paymentExpiresAt := result.ExpiresAt.UTC()
	if !paymentExpiresAt.After(now) {
		return fmt.Errorf("%w: expires_at must be strictly after validation time", ErrPaymentExpiryNotUsable)
	}

	if check.LatestAllowedExpiresAt != nil {
		latest := check.LatestAllowedExpiresAt.UTC()
		if paymentExpiresAt.After(latest) {
			return fmt.Errorf("%w: expires_at is after the allowed payment window", ErrPaymentExpiryNotUsable)
		}
	}

	if err := ValidatePaymentRedirectURL(result.PaymentLink, maxURL); err != nil {
		return err
	}

	return nil
}
