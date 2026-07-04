package financialcorepaymentsessionuc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	paymentSessionClockSkew     = 5 * time.Minute
	maxPaymentSessionIdentifier = 256
	maxPaymentSessionURLLength  = 2048
	maxPaymentSessionEnvelopeID = 256
)

func validateMinimumTrustedEnvelope(input ProcessInput) error {
	const op = "financialcorepaymentsessionuc.validateMinimumTrustedEnvelope"

	if input.EventID == "" || len(input.EventID) > maxPaymentSessionEnvelopeID {
		return eventIDInvalidError()
	}

	if input.SourceService == "" || len(input.SourceService) > maxPaymentSessionIdentifier {
		return sourceServiceInvalidError()
	}

	if input.EventType == "" || len(input.EventType) > maxPaymentSessionIdentifier {
		return eventTypeInvalidError()
	}

	if input.EventVersion == "" || len(input.EventVersion) > 32 {
		return eventVersionInvalidError()
	}

	if len(input.RawPayload) == 0 {
		return rawPayloadRequiredError()
	}

	if input.EventType != installmentPaymentSessionCreatedEventType {
		return unsupportedEventTypeError(fmt.Errorf("%s: unsupported event type %q", op, input.EventType))
	}

	if input.EventVersion != expectedEventVersion {
		return unsupportedEventVersionError(fmt.Errorf("%s: unsupported event version %q", op, input.EventVersion))
	}

	if input.SourceService != expectedSourceService {
		return unsupportedSourceServiceError(fmt.Errorf("%s: unsupported source service %q", op, input.SourceService))
	}

	return nil
}

func validateBusinessInput(input ProcessInput, now time.Time) error {
	if input.BNPLPaymentAttemptID.IsZero() {
		return bnplPaymentAttemptIDRequiredError()
	}

	if input.FinancialPaymentIntentID == "" || len(input.FinancialPaymentIntentID) > maxPaymentSessionIdentifier {
		return financialPaymentIntentIDInvalidError()
	}

	if input.FinancialPaymentSessionID == "" || len(input.FinancialPaymentSessionID) > maxPaymentSessionIdentifier {
		return financialPaymentSessionIDInvalidError()
	}

	if input.FinancialPaymentAttemptID != nil && len(*input.FinancialPaymentAttemptID) > maxPaymentSessionIdentifier {
		return financialPaymentAttemptIDTooLongError()
	}

	if err := model.ValidatePositiveAmountMinor(input.AmountMinor); err != nil {
		return amountMinorInvalidError(err)
	}

	if !input.Currency.IsValid() {
		return currencyInvalidError()
	}

	if input.EventOccurredAt.IsZero() {
		return eventOccurredAtRequiredError()
	}

	if input.EventOccurredAt.After(now.Add(paymentSessionClockSkew)) {
		return eventOccurredAtTooFarInFutureError()
	}

	if input.ExpiresAt.IsZero() || !input.ExpiresAt.After(input.EventOccurredAt) {
		return paymentSessionExpiryInvalidError()
	}

	if err := validatePaymentSessionURL(input.PaymentURL); err != nil {
		return err
	}

	return nil
}

func validateUsablePaymentSession(input ProcessInput, now time.Time) error {
	if !input.ExpiresAt.After(now) {
		return paymentSessionExpiredError()
	}

	return nil
}

func validatePaymentSessionURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxPaymentSessionURLLength {
		return paymentURLInvalidError(errors.New("payment url is required and must be within the supported length"))
	}

	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" || parsed.User != nil {
		cause := err
		if cause == nil {
			cause = errors.New("payment url must be an absolute https url without user information")
		}

		return paymentURLMustBeAbsoluteHTTPSError(cause)
	}

	return nil
}

func validateAttemptMatchesInput(attempt model.PaymentAttempt, input ProcessInput) error {
	if attempt.ID != input.BNPLPaymentAttemptID {
		return paymentAttemptIDMismatchConflictError()
	}

	if attempt.Purpose != model.InstallmentRepaymentFinancialCorePaymentPurpose {
		return paymentAttemptPurposeConflictError()
	}

	if attempt.AmountMinor != input.AmountMinor {
		return paymentAttemptAmountMismatchConflictError()
	}

	if attempt.Currency != input.Currency {
		return paymentAttemptCurrencyMismatchConflictError()
	}

	return nil
}

func sessionIdentityMatches(attempt model.PaymentAttempt, input ProcessInput) bool {
	if attempt.FinancialPaymentIntentID == nil || *attempt.FinancialPaymentIntentID != input.FinancialPaymentIntentID {
		return false
	}

	if attempt.FinancialPaymentSessionID == nil || *attempt.FinancialPaymentSessionID != input.FinancialPaymentSessionID {
		return false
	}

	if input.FinancialPaymentAttemptID != nil &&
		(attempt.FinancialPaymentAttemptID == nil || *attempt.FinancialPaymentAttemptID != *input.FinancialPaymentAttemptID) {
		return false
	}

	if attempt.PaymentURL == nil || strings.TrimSpace(*attempt.PaymentURL) != input.PaymentURL {
		return false
	}

	if attempt.FinancialPaymentExpiresAt == nil || !attempt.FinancialPaymentExpiresAt.Equal(input.ExpiresAt) {
		return false
	}

	return true
}

func optionalPaymentURL(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	return &value
}

func optionalExpiresAt(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}

	utc := value.UTC()
	return &utc
}

func optionalProviderAttemptID(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func inboxSubjectForPaymentAttempt(attemptID model.PaymentAttemptID) (subjectType *string, subjectID *string) {
	st := string(model.PaymentAttemptInboxSubjectType)
	sid := attemptID.String()
	return &st, &sid
}

func inboxExternalReference(input ProcessInput) (*string, *string) {
	if input.FinancialPaymentIntentID == "" {
		return nil, nil
	}

	extRefType := string(model.FinancialPaymentIntentInboxExternalReferenceType)
	intentID := input.FinancialPaymentIntentID
	return &extRefType, &intentID
}

type canonicalPaymentSessionFingerprint struct {
	SourceService             string `json:"source_service"`
	EventID                   string `json:"event_id"`
	EventType                 string `json:"event_type"`
	EventVersion              string `json:"event_version"`
	BNPLPaymentAttemptID      string `json:"bnpl_payment_attempt_id"`
	FinancialPaymentIntentID  string `json:"financial_payment_intent_id"`
	FinancialPaymentAttemptID string `json:"financial_payment_attempt_id,omitempty"`
	FinancialPaymentSessionID string `json:"financial_payment_session_id"`
	PaymentURL                string `json:"payment_url,omitempty"`
	AmountMinor               uint64 `json:"amount_minor"`
	Currency                  string `json:"currency"`
	ExpiresAt                 string `json:"expires_at,omitempty"`
	EventOccurredAt           string `json:"event_occurred_at,omitempty"`
}

func (uc *UseCase) paymentSessionPayloadFingerprint(input ProcessInput) (string, error) {
	const op = "financialcorepaymentsessionuc.UseCase.paymentSessionPayloadFingerprint"

	canonical := canonicalPaymentSessionFingerprint{
		SourceService:             input.SourceService,
		EventID:                   input.EventID,
		EventType:                 input.EventType,
		EventVersion:              input.EventVersion,
		BNPLPaymentAttemptID:      input.BNPLPaymentAttemptID.String(),
		FinancialPaymentIntentID:  input.FinancialPaymentIntentID,
		FinancialPaymentSessionID: input.FinancialPaymentSessionID,
		PaymentURL:                input.PaymentURL,
		AmountMinor:               input.AmountMinor,
		Currency:                  input.Currency.String(),
	}

	if input.FinancialPaymentAttemptID != nil {
		canonical.FinancialPaymentAttemptID = *input.FinancialPaymentAttemptID
	}

	if !input.ExpiresAt.IsZero() {
		canonical.ExpiresAt = input.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}

	if !input.EventOccurredAt.IsZero() {
		canonical.EventOccurredAt = input.EventOccurredAt.UTC().Format(time.RFC3339Nano)
	}

	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("%s: marshal canonical payment session fingerprint: %w", op, err)
	}

	const versionPrefix = "financial-core-installment-payment-session:v1:"
	value, err := uc.fingerprintHasher.Hash(versionPrefix + string(raw))
	if err != nil {
		return "", fmt.Errorf("%s: hash fingerprint: %w", op, err)
	}

	if err := value.Validate(); err != nil {
		return "", fmt.Errorf("%s: validate fingerprint: %w", op, err)
	}

	return value.String(), nil
}
