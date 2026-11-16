package field

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Stable attribute keys. Prefer these constants over raw strings.
const (
	KeyEvent         = "event"
	KeyTraceID       = "trace_id"
	KeyRequestID     = "request_id"
	KeyCorrelationID = "correlation_id"

	KeySessionID          = "session_id"
	KeyCustomerID         = "customer_id"
	KeyExternalCustomerID = "external_customer_id"
	KeyExternalSubjectID  = "external_subject_id"
	KeyMerchantProfileID  = "merchant_profile_id"
	KeyExternalMerchantID = "external_merchant_id"
	KeyMerchantBranchID   = "merchant_branch_id"
	KeyContractID         = "contract_id"
	KeyPaymentAttemptID   = "payment_attempt_id"
	KeyInstallmentID      = "installment_id"
	KeyDecisionID         = "decision_id"
	KeyOTPChallengeID     = "otp_challenge_id"
	KeySagaID             = "saga_id"
	KeySagaStep           = "saga_step"
	KeyOutboxMessageID    = "outbox_message_id"
	KeyInboxMessageID     = "inbox_message_id"
	KeyEventID            = "event_id"
	KeyCausationID        = "causation_id"
	KeyJobName            = "job_name"
	KeyBatchSize          = "batch_size"
	KeyProcessedCount     = "processed_count"
	KeyFailedCount        = "failed_count"
	KeyAttemptCount       = "attempt_count"
	KeyDurationMS         = "duration_ms"
	KeyOutcome            = "outcome"
	KeyReasonCode         = "reason_code"
	KeyErrorCode          = "error_code"
	KeyStatus             = "status"
	KeyCreditType         = "credit_type"
	KeyAmountMinor        = "amount_minor"
	KeyCurrency           = "currency"
	KeyRecipientPhone     = "recipient_phone"
	KeyTemplateID         = "template_id"
	KeyProvider           = "provider"
	KeyTopic              = "topic"
	KeyComponent          = "component"
	KeyTaskName           = "task_name"
	KeyScheduleID         = "schedule_id"
	KeyTaskCount          = "task_count"
	KeyError              = "error"
	KeyReason             = "reason"
	KeySQL                = "sql"
	KeyVersion            = "version"
	KeyMigration          = "migration"
	KeyKeyID              = "key_id"
	KeyRowNumber          = "row_number"
	KeyMobileHash         = "mobile_hash"
	KeyExpectedStatus     = "expected_status"
	KeyActualStatus       = "actual_status"
	KeyHTTPMethod         = "http.method"
	KeyHTTPRoute          = "http.route"
	KeyHTTPStatusCode     = "http.status_code"
	KeyHTTPRequestSize    = "http.request_size"
	KeyHTTPResponseSize   = "http.response_size"
	KeyHTTPLatency        = "http.latency"
	KeyHTTPAborted        = "http.aborted"
	KeyHTTPResponseBody   = "http.response_body"
)

// attr is the concrete Field produced by constructors.
type attr struct {
	key   string
	value any
}

func (a attr) Key() string { return a.key }
func (a attr) Value() any  { return a.value }

func newAttr(key string, value any) log.Field {
	return attr{key: key, value: value}
}

// SessionID attaches a checkout session identifier.
func SessionID(id model.CheckoutSessionID) log.Field {
	return newAttr(KeySessionID, id.String())
}

// CustomerID attaches an internal customer profile identifier.
func CustomerID(id model.CustomerID) log.Field {
	return newAttr(KeyCustomerID, id.String())
}

// ExternalCustomerID attaches a CRS external customer identifier.
func ExternalCustomerID(id model.ExternalCustomerID) log.Field {
	return newAttr(KeyExternalCustomerID, id.String())
}

// ExternalSubjectID attaches an Identity Service subject identifier.
func ExternalSubjectID(id model.ExternalSubjectID) log.Field {
	return newAttr(KeyExternalSubjectID, id.String())
}

// MerchantProfileID attaches an internal merchant profile identifier.
func MerchantProfileID(id model.MerchantProfileID) log.Field {
	return newAttr(KeyMerchantProfileID, id.String())
}

// ExternalMerchantID attaches an external merchant identifier.
func ExternalMerchantID(id model.ExternalMerchantID) log.Field {
	return newAttr(KeyExternalMerchantID, id.String())
}

// MerchantBranchID attaches a merchant branch profile identifier.
func MerchantBranchID(id model.MerchantBranchProfileID) log.Field {
	return newAttr(KeyMerchantBranchID, id.String())
}

// ContractID attaches a contract identifier.
func ContractID(id model.ContractID) log.Field {
	return newAttr(KeyContractID, id.String())
}

// PaymentAttemptID attaches a payment attempt identifier.
func PaymentAttemptID(id model.PaymentAttemptID) log.Field {
	return newAttr(KeyPaymentAttemptID, id.String())
}

// InstallmentID attaches an installment identifier.
func InstallmentID(id model.InstallmentID) log.Field {
	return newAttr(KeyInstallmentID, id.String())
}

// DecisionID attaches a decision identifier.
func DecisionID(id model.DecisionID) log.Field {
	return newAttr(KeyDecisionID, id.String())
}

// OTPChallengeID attaches an OTP challenge identifier (never the OTP itself).
func OTPChallengeID(id model.OTPChallengeID) log.Field {
	return newAttr(KeyOTPChallengeID, id.String())
}

// SagaID attaches a saga instance identifier.
func SagaID(id string) log.Field {
	return newAttr(KeySagaID, id)
}

// SagaStep attaches a saga step name.
func SagaStep(step string) log.Field {
	return newAttr(KeySagaStep, step)
}

// OutboxMessageID attaches an outbox message identifier.
func OutboxMessageID(id string) log.Field {
	return newAttr(KeyOutboxMessageID, id)
}

// InboxMessageID attaches an inbox message identifier.
func InboxMessageID(id string) log.Field {
	return newAttr(KeyInboxMessageID, id)
}

// EventID attaches an integration or messaging event identifier.
func EventID(id string) log.Field {
	return newAttr(KeyEventID, id)
}

// CausationID attaches a causation identifier linking related async work.
func CausationID(id string) log.Field {
	return newAttr(KeyCausationID, id)
}

// JobName attaches a background job name.
func JobName(name string) log.Field {
	return newAttr(KeyJobName, name)
}

// BatchSize attaches a processing batch size.
func BatchSize(n int) log.Field {
	return newAttr(KeyBatchSize, n)
}

// ProcessedCount attaches a successful processing count.
func ProcessedCount(n int) log.Field {
	return newAttr(KeyProcessedCount, n)
}

// FailedCount attaches a failed processing count.
func FailedCount(n int) log.Field {
	return newAttr(KeyFailedCount, n)
}

// AttemptCount attaches a retry or attempt counter.
func AttemptCount(n int) log.Field {
	return newAttr(KeyAttemptCount, n)
}

// Duration attaches an elapsed duration as milliseconds.
func Duration(d time.Duration) log.Field {
	return newAttr(KeyDurationMS, d.Milliseconds())
}

// Outcome attaches a stable outcome label (for example "approved", "replayed").
func Outcome(outcome string) log.Field {
	return newAttr(KeyOutcome, outcome)
}

// ReasonCode attaches a stable semantic reason code (not a free-form error).
func ReasonCode(code string) log.Field {
	return newAttr(KeyReasonCode, code)
}

// ErrorCode attaches a stable business or infrastructure error code.
func ErrorCode(code string) log.Field {
	return newAttr(KeyErrorCode, code)
}

// Status attaches a domain or processing status string.
func Status(status string) log.Field {
	return newAttr(KeyStatus, status)
}

// CreditType attaches a credit product or limit type.
func CreditType(creditType string) log.Field {
	return newAttr(KeyCreditType, creditType)
}

// AmountMinor attaches a monetary amount in minor units.
func AmountMinor(amount int64) log.Field {
	return newAttr(KeyAmountMinor, amount)
}

// Currency attaches an ISO currency code.
func Currency(code string) log.Field {
	return newAttr(KeyCurrency, code)
}

// RecipientPhone attaches a redacted phone number suitable for production logs.
//
// Full numbers are never emitted. Invalid or empty values are fully redacted.
func RecipientPhone(phone string) log.Field {
	return newAttr(KeyRecipientPhone, RedactPhone(phone))
}

// TemplateID attaches a notification template identifier.
func TemplateID(id string) log.Field {
	return newAttr(KeyTemplateID, id)
}

// Provider attaches an external provider name.
func Provider(name string) log.Field {
	return newAttr(KeyProvider, name)
}

// Topic attaches a messaging topic name.
func Topic(topic string) log.Field {
	return newAttr(KeyTopic, topic)
}

// Component attaches a shutdown or runtime component name.
func Component(name string) log.Field {
	return newAttr(KeyComponent, name)
}

// TaskName attaches a scheduled task name.
func TaskName(name string) log.Field {
	return newAttr(KeyTaskName, name)
}

// ErrorMessage attaches a bounded diagnostic error string.
//
// Prefer ErrorCode / ReasonCode for business failures. Avoid passing raw
// provider payloads or secrets.
func ErrorMessage(message string) log.Field {
	return newAttr(KeyError, truncate(message, 512))
}

// Reason attaches a short free-form operational reason (prefer ReasonCode).
func Reason(reason string) log.Field {
	return newAttr(KeyReason, truncate(reason, 256))
}

// allowlist is the set of keys accepted by Sanitize.
var allowlist = map[string]struct{}{
	KeyEvent: {}, KeyTraceID: {}, KeyRequestID: {}, KeyCorrelationID: {},
	KeySessionID: {}, KeyCustomerID: {}, KeyExternalCustomerID: {}, KeyExternalSubjectID: {},
	KeyMerchantProfileID: {}, KeyExternalMerchantID: {}, KeyMerchantBranchID: {},
	KeyContractID: {}, KeyPaymentAttemptID: {}, KeyInstallmentID: {}, KeyDecisionID: {},
	KeyOTPChallengeID: {}, KeySagaID: {}, KeySagaStep: {},
	KeyOutboxMessageID: {}, KeyInboxMessageID: {}, KeyEventID: {}, KeyCausationID: {},
	KeyJobName: {}, KeyBatchSize: {}, KeyProcessedCount: {}, KeyFailedCount: {},
	KeyAttemptCount: {}, KeyDurationMS: {}, KeyOutcome: {}, KeyReasonCode: {},
	KeyErrorCode: {}, KeyStatus: {}, KeyCreditType: {}, KeyAmountMinor: {}, KeyCurrency: {},
	KeyRecipientPhone: {}, KeyTemplateID: {}, KeyProvider: {}, KeyTopic: {},
	KeyComponent: {}, KeyTaskName: {}, KeyScheduleID: {}, KeyTaskCount: {},
	KeyError: {}, KeyReason: {}, KeySQL: {}, KeyVersion: {}, KeyMigration: {}, KeyKeyID: {},
	KeyRowNumber: {}, KeyMobileHash: {}, KeyExpectedStatus: {}, KeyActualStatus: {},
	KeyHTTPMethod: {}, KeyHTTPRoute: {}, KeyHTTPStatusCode: {}, KeyHTTPRequestSize: {},
	KeyHTTPResponseSize: {}, KeyHTTPLatency: {}, KeyHTTPAborted: {}, KeyHTTPResponseBody: {},
}

// Sanitize validates a Field against the allowlist and safe value kinds.
//
// Unknown keys and unsafe values are rejected (ok=false) so they are never
// emitted. RecipientPhone values are re-redacted defensively.
func Sanitize(f log.Field) (key string, value any, ok bool) {
	if f == nil {
		return "", nil, false
	}

	key = strings.TrimSpace(f.Key())
	if key == "" {
		return "", nil, false
	}

	if _, allowed := allowlist[key]; !allowed {
		return "", nil, false
	}

	value, ok = normalizeValue(key, f.Value())
	return key, value, ok
}

func normalizeValue(key string, value any) (any, bool) {
	if value == nil {
		return nil, true
	}

	switch v := value.(type) {
	case string:
		if key == KeyRecipientPhone {
			return RedactPhone(v), true
		}

		return v, true

	case bool:
		return v, true

	case int:
		return v, true

	case int8:
		return int(v), true

	case int16:
		return int(v), true

	case int32:
		return int(v), true

	case int64:
		return v, true

	case uint:
		return v, true

	case uint8:
		return uint(v), true

	case uint16:
		return uint(v), true

	case uint32:
		return uint(v), true

	case uint64:
		return v, true

	case float32:
		return float64(v), true

	case float64:
		return v, true

	case time.Duration:
		return v.Milliseconds(), true

	case fmt.Stringer:
		return v.String(), true

	case map[string]any:
		// Access-log response body projections are pre-filtered maps.
		if key == KeyHTTPResponseBody {
			return v, true
		}

		return nil, false

	default:
		return nil, false
	}
}

// RedactPhone masks a phone number for production logs.
//
// Valid E.164-like values keep a short prefix hint and the last four digits.
// Unknown or invalid formats are fully redacted. Already-redacted values are
// returned unchanged so Sanitize can re-apply defensively without collapsing
// them.
func RedactPhone(phone string) string {
	trimmed := strings.TrimSpace(phone)
	if trimmed == "" {
		return "[redacted]"
	}

	if trimmed == "[redacted]" ||
		strings.HasPrefix(trimmed, "+***") ||
		strings.HasPrefix(trimmed, "***") {
		return trimmed
	}

	digits := make([]rune, 0, len(trimmed))
	for _, r := range trimmed {
		if unicode.IsDigit(r) {
			digits = append(digits, r)
		}
	}

	if len(digits) < 8 || len(digits) > 15 {
		return "[redacted]"
	}

	last := string(digits[len(digits)-4:])
	if strings.HasPrefix(trimmed, "+") {
		return "+***" + last
	}

	return "***" + last
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}

	return s[:max]
}
