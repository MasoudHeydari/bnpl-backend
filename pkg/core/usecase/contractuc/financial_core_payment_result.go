package contractuc

import (
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const financialCoreContractSourceType = "bnpl_contract"

func validatePaymentOccurredInTime(contractRow model.Contract, paidAt time.Time) error {
	const op = "contractuc.validatePaymentOccurredInTime"

	if paidAt.IsZero() {
		return paidAtRequiredError()
	}

	if !contractRow.PaymentOccurredOnTime(paidAt) {
		return newReconciliationError(
			model.PaymentAfterDeadlinePaymentReconciliationReason,
			fmt.Errorf("%s: payment occurred after contract payment deadline", op),
		)
	}

	return nil
}

// validateCheckoutPaymentAttemptIdentity validates the inbound successful payment against
// the durable checkout PaymentAttempt. The attempt is authoritative for retries; Contract
// session fields may still describe an earlier failed/canceled attempt until the successful
// activation transaction replaces their identity.
func validateCheckoutPaymentAttemptIdentity(
	contractRow model.Contract,
	attempt model.PaymentAttempt,
	input ActivateFromFinancialCorePaymentResultInput,
) error {
	const op = "contractuc.validateCheckoutPaymentAttemptIdentity"

	if attempt.Purpose != model.CheckoutFirstInstallmentFinancialCorePaymentPurpose ||
		attempt.ContractID != contractRow.ID {
		return newReconciliationError(
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: checkout payment attempt does not belong to contract", op),
		)
	}

	if err := requireStoredIdentifierMatch(
		attempt.FinancialPaymentIntentID,
		input.PaymentIntentID,
		model.PaymentReferenceMismatchPaymentReconciliationReason,
		"payment_intent_id",
	); err != nil {
		return err
	}

	if err := requireStoredIdentifierMatch(
		attempt.FinancialPaymentSessionID,
		input.PaymentSessionID,
		model.PaymentReferenceMismatchPaymentReconciliationReason,
		"payment_session_id",
	); err != nil {
		return err
	}

	if err := requireCompatibleProviderAttemptID(
		attempt.FinancialPaymentAttemptID,
		input.FinancialPaymentAttemptID,
	); err != nil {
		return err
	}

	// When Contract already points to this same payment intent, its invoice/session
	// metadata must also agree. A different stored intent belongs to an older retry and
	// is replaced transactionally during activation.
	if contractRow.FinancialPaymentIntentID != nil &&
		strings.TrimSpace(*contractRow.FinancialPaymentIntentID) == strings.TrimSpace(input.PaymentIntentID) {
		if err := requireStoredIdentifierMatch(
			contractRow.FinancialInvoiceID,
			input.InvoiceID,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			"invoice_id",
		); err != nil {
			return err
		}

		if err := requireStoredIdentifierMatch(
			contractRow.FinancialPaymentSessionID,
			input.PaymentSessionID,
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			"payment_session_id",
		); err != nil {
			return err
		}
	}

	return nil
}

// validateCheckoutAlreadyAppliedIdentity proves the stored payment identity of an already
// applied checkout contract represents the same successful payment. Unlike initial activation
// compatibility, a missing stored payment intent never matches an incoming payment: the
// duplicate-applied path requires positive proof of the original payment intent. Invoice and
// session are validated per the temporary integration contract; provider-attempt IDs stay
// compatible when either side is absent, but two conflicting non-empty values reconcile.
func validateCheckoutAlreadyAppliedIdentity(
	contractRow model.Contract,
	input ActivateFromFinancialCorePaymentResultInput,
) error {
	if err := requireStoredPaymentIntentProof(contractRow, input); err != nil {
		return err
	}

	if err := requireStoredIdentifierMatch(
		contractRow.FinancialInvoiceID,
		input.InvoiceID,
		model.PaymentReferenceMismatchPaymentReconciliationReason,
		"invoice_id",
	); err != nil {
		return err
	}

	if err := requireStoredIdentifierMatch(
		contractRow.FinancialPaymentSessionID,
		input.PaymentSessionID,
		model.PaymentReferenceMismatchPaymentReconciliationReason,
		"payment_session_id",
	); err != nil {
		return err
	}

	return requireCompatibleProviderAttemptID(
		contractRow.SuccessfulFinancialPaymentAttemptID,
		input.FinancialPaymentAttemptID,
	)
}

// requireStoredPaymentIntentProof requires the stored payment intent to be present, non-empty,
// and equal to the incoming payment intent.
func requireStoredPaymentIntentProof(
	contractRow model.Contract,
	input ActivateFromFinancialCorePaymentResultInput,
) error {
	const op = "contractuc.requireStoredPaymentIntentProof"

	if contractRow.FinancialPaymentIntentID == nil ||
		strings.TrimSpace(*contractRow.FinancialPaymentIntentID) == "" {
		return newReconciliationError(
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: active contract has no stored financial payment intent to match", op),
		)
	}

	if strings.TrimSpace(*contractRow.FinancialPaymentIntentID) != strings.TrimSpace(input.PaymentIntentID) {
		return newReconciliationError(
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: financial payment intent does not match stored contract payment intent", op),
		)
	}

	return nil
}

// requireCompatibleProviderAttemptID treats an absent value on either side as compatible while
// two conflicting non-empty provider-attempt IDs require reconciliation. Provider-attempt ID
// is optional audit metadata and never replaces payment intent as the primary correlation.
func requireCompatibleProviderAttemptID(stored *string, incoming *string) error {
	const op = "contractuc.requireCompatibleProviderAttemptID"

	storedID := normalizeOptionalProviderAttemptID(stored)
	incomingID := normalizeOptionalProviderAttemptID(incoming)

	if storedID == nil || incomingID == nil {
		return nil
	}

	if *storedID != *incomingID {
		return newReconciliationError(
			model.PaymentReferenceMismatchPaymentReconciliationReason,
			fmt.Errorf("%s: financial provider-attempt id conflicts with stored successful attempt id", op),
		)
	}

	return nil
}

func requireStoredIdentifierMatch(
	stored *string,
	value string,
	reason model.PaymentReconciliationReason,
	field string,
) error {
	const op = "contractuc.requireStoredIdentifierMatch"

	if stored == nil || strings.TrimSpace(*stored) == "" {
		return nil
	}

	if *stored != value {
		return newReconciliationError(
			reason,
			fmt.Errorf("%s: financial core %s does not match stored payment session", op, field),
		)
	}

	return nil
}

// normalizeOptionalProviderAttemptID trims an optional Financial Core provider-attempt id,
// returning nil so absence and empty strings persist as SQL NULL.
func normalizeOptionalProviderAttemptID(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func resolveContractID(input ActivateFromFinancialCorePaymentResultInput) (model.ContractID, error) {
	if !input.ContractID.IsZero() {
		return input.ContractID, nil
	}

	if !strings.EqualFold(strings.TrimSpace(input.SourceType), financialCoreContractSourceType) {
		return model.ContractID{}, paymentResultSourceTypeInvalidError()
	}

	contractID, err := model.ParseContractID(strings.TrimSpace(input.SourceID))
	if err != nil || contractID.IsZero() {
		return model.ContractID{}, paymentResultSourceIDInvalidError()
	}

	return contractID, nil
}

func validateAllocationPaidInTime(allocation model.CheckoutDecisionAllocation, paidAt time.Time) error {
	const op = "contractuc.validateAllocationPaidInTime"

	if paidAt.UTC().After(allocation.ExpiresAt.UTC()) {
		return newReconciliationError(
			model.PaymentAfterDeadlinePaymentReconciliationReason,
			fmt.Errorf("%s: payment occurred after checkout decision allocation expired", op),
		)
	}

	return nil
}

func validateExpiredContractPayment(contractRow model.Contract, paidAt time.Time) error {
	const op = "contractuc.validateExpiredContractPayment"

	if contractRow.PaymentOccurredOnTime(paidAt) {
		return newReconciliationError(
			model.ContractExpiredPaymentWasOnTimePaymentReconciliationReason,
			fmt.Errorf("%s: %w: contract expired but payment occurred before deadline", op, ErrPaymentRequiresReconciliation),
		)
	}

	return newReconciliationError(
		model.PaymentAfterDeadlinePaymentReconciliationReason,
		fmt.Errorf("%s: payment occurred after contract payment deadline", op),
	)
}
