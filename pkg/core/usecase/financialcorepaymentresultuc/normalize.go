package financialcorepaymentresultuc

import (
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// normalizeProcessInput trims integration identity fields once so the same values are used
// for inbox persistence, fingerprinting, conflict detection, and outbound acknowledgements.
func normalizeProcessInput(input ProcessInput) ProcessInput {
	input.EventID = strings.TrimSpace(input.EventID)
	input.EventType = strings.TrimSpace(input.EventType)
	input.EventVersion = strings.TrimSpace(input.EventVersion)
	input.SourceService = strings.TrimSpace(input.SourceService)
	input.PaymentPurpose = model.FinancialCorePaymentPurpose(strings.TrimSpace(string(input.PaymentPurpose)))
	input.InvoiceID = strings.TrimSpace(input.InvoiceID)
	input.PaymentIntentID = strings.TrimSpace(input.PaymentIntentID)
	input.PaymentAttemptID = strings.TrimSpace(input.PaymentAttemptID)
	input.PaymentSessionID = strings.TrimSpace(input.PaymentSessionID)
	input.RawPaymentStatus = strings.TrimSpace(input.RawPaymentStatus)
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.SourceID = strings.TrimSpace(input.SourceID)
	input.Currency = model.Currency(strings.TrimSpace(input.Currency.String()))

	if input.CorrelationID != nil {
		value := strings.TrimSpace(*input.CorrelationID)
		input.CorrelationID = &value
	}

	if input.CausationID != nil {
		value := strings.TrimSpace(*input.CausationID)
		input.CausationID = &value
	}

	if input.FailureCode != nil {
		value := strings.TrimSpace(*input.FailureCode)
		input.FailureCode = &value
	}

	return input
}

func optionalProviderAttemptID(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

// canonicalPaymentStatus returns the deterministic status representation used for
// fingerprinting, durable failure metadata, logging, and process diagnostics.
//
// Known statuses always use the typed canonical value so equivalent raw spellings such
// as "Succeeded" and "succeeded" produce one fingerprint. Unknown statuses retain their
// distinct normalized value.
func canonicalPaymentStatus(input ProcessInput) string {
	if input.PaymentStatus.IsValid() {
		return string(input.PaymentStatus)
	}

	return normalizeUnknownPaymentStatus(input.RawPaymentStatus)
}

// normalizeUnknownPaymentStatus applies the same trim and case policy the adapter uses
// when mapping known statuses, without collapsing distinct unknown values.
func normalizeUnknownPaymentStatus(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func unsupportedPaymentStatusMessage(input ProcessInput) string {
	if status := canonicalPaymentStatus(input); status != "" {
		return fmt.Sprintf("unsupported payment status %q", status)
	}

	return "unsupported payment status"
}
