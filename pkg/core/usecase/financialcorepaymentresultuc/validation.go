package financialcorepaymentresultuc

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

func isContractNotFoundError(err error) bool {
	return cerr.IsCode(err, cerr.CodeContractNotFound) ||
		cerr.IsCode(err, cerr.CodeResourceNotFound)
}

func isInstallmentNotFoundError(err error) bool {
	return cerr.IsCode(err, cerr.CodeInstallmentNotFound) ||
		cerr.IsCode(err, cerr.CodeResourceNotFound)
}

const (
	expectedPaymentResultEventType     = "financial_core.payment_result.v1"
	expectedPaymentResultEventVersion  = "1"
	expectedPaymentResultSourceService = "financial-core"
	paymentResultClockSkew             = 5 * time.Minute
	maxPaymentResultIdentifierLength   = 256
	maxPaymentResultFailureCodeLength  = 256
)

func validateMinimumTrustedEnvelope(input ProcessInput) error {
	const op = "financialcorepaymentresultuc.validateMinimumTrustedEnvelope"

	if input.EventID == "" || len(input.EventID) > maxPaymentResultIdentifierLength {
		return eventIDInvalidError()
	}

	if input.SourceService == "" || len(input.SourceService) > maxPaymentResultIdentifierLength {
		return sourceServiceInvalidError()
	}

	if input.SourceService != expectedPaymentResultSourceService {
		return unsupportedSourceServiceError(
			fmt.Errorf("%s: unsupported payment result source service %q", op, input.SourceService),
		)
	}

	if input.EventType == "" || len(input.EventType) > maxPaymentResultIdentifierLength {
		return eventTypeInvalidError()
	}

	if input.EventType != expectedPaymentResultEventType {
		return unsupportedEventTypeError(
			fmt.Errorf("%s: unsupported payment result event type %q", op, input.EventType),
		)
	}

	if input.EventVersion == "" || len(input.EventVersion) > 32 {
		return eventVersionInvalidError()
	}

	if input.EventVersion != expectedPaymentResultEventVersion {
		return unsupportedEventVersionError(
			fmt.Errorf("%s: unsupported payment result event version %q", op, input.EventVersion),
		)
	}

	if len(input.RawPayload) == 0 {
		return rawPayloadRequiredError()
	}

	return nil
}

func validatePaymentResultTemporalInput(input ProcessInput, now time.Time) error {
	if input.EventOccurredAt.IsZero() {
		return eventOccurredAtRequiredError()
	}

	if input.EventOccurredAt.After(now.Add(paymentResultClockSkew)) {
		return eventOccurredAtTooFarInFutureError()
	}

	for field, value := range map[string]string{
		"invoice_id":         input.InvoiceID,
		"payment_intent_id":  input.PaymentIntentID,
		"payment_attempt_id": input.PaymentAttemptID,
		"payment_session_id": input.PaymentSessionID,
		"source_type":        input.SourceType,
		"source_id":          input.SourceID,
	} {
		if len(value) > maxPaymentResultIdentifierLength {
			return identifierTooLongError(field)
		}
	}

	if input.FailureCode != nil && len(*input.FailureCode) > maxPaymentResultFailureCodeLength {
		return failureCodeTooLongError()
	}

	if err := model.ValidateNonNegativeAmountMinor(input.PaidAmountMinor); err != nil {
		return amountMinorInvalidError(err)
	}

	if input.PaymentStatus == model.SucceededFinancialPaymentResultStatus {
		if input.PaidAt.IsZero() {
			return paidAtRequiredError()
		}

		if input.PaidAt.After(now.Add(paymentResultClockSkew)) {
			return paidAtTooFarInFutureError()
		}

		if input.PaidAt.After(input.EventOccurredAt.Add(paymentResultClockSkew)) {
			return paidAtAfterEventError()
		}

		if !input.Currency.IsValid() {
			return currencyInvalidError()
		}
	}

	return nil
}

func validateCheckoutPaymentResultEnvelope(input contractuc.ActivateFromFinancialCorePaymentResultInput) error {
	if input.ContractID.IsZero() &&
		(input.SourceType == "" || input.SourceID == "") {
		return contractOrSourceRequiredError()
	}

	if input.PaymentIntentID == "" {
		return paymentIntentIDRequiredError()
	}

	if input.EventID == "" || len(input.EventID) > maxPaymentResultIdentifierLength {
		return eventIDInvalidError()
	}

	if input.EventOccurredAt.IsZero() {
		return eventOccurredAtRequiredError()
	}

	return nil
}

func validateSucceededCheckoutPaymentMetadata(input contractuc.ActivateFromFinancialCorePaymentResultInput) error {
	if input.InvoiceID == "" {
		return invoiceIDRequiredError()
	}

	if input.PaymentSessionID == "" {
		return paymentSessionIDRequiredError()
	}

	if err := model.ValidatePositiveAmountMinor(input.PaidAmountMinor); err != nil {
		return amountMinorInvalidError(err)
	}

	if !input.Currency.IsValid() {
		return currencyInvalidError()
	}

	if input.PaidAt.IsZero() {
		return paidAtRequiredError()
	}

	return nil
}

func validateCheckoutPaymentResultStatus(status model.FinancialPaymentResultStatus) error {
	if status == "" {
		return nil
	}

	if !status.IsValid() {
		return paymentStatusUnsupportedError()
	}

	return nil
}

func validateInstallmentRepaymentBusinessInput(input ProcessInput) error {
	if input.PaymentIntentID == "" {
		return paymentIntentIDRequiredError()
	}

	if input.PaymentStatus == model.SucceededFinancialPaymentResultStatus {
		if err := model.ValidatePositiveAmountMinor(input.PaidAmountMinor); err != nil {
			return amountMinorInvalidError(err)
		}

		if !input.Currency.IsValid() {
			return currencyInvalidError()
		}

		if input.PaidAt.IsZero() {
			return paidAtRequiredError()
		}
	}

	return nil
}

func resolveContractID(input contractuc.ActivateFromFinancialCorePaymentResultInput) (model.ContractID, error) {
	if !input.ContractID.IsZero() {
		return input.ContractID, nil
	}

	if !strings.EqualFold(strings.TrimSpace(input.SourceType), financialCoreContractSourceType) {
		return model.ContractID{}, sourceTypeNotContractError(nil)
	}

	contractID, err := model.ParseContractID(strings.TrimSpace(input.SourceID))
	if err != nil || contractID.IsZero() {
		return model.ContractID{}, sourceIDInvalidError(err)
	}

	return contractID, nil
}

func validateFinancialPaymentIntentMatchesAttempt(
	attempt model.PaymentAttempt,
	input ProcessInput,
) error {
	if attempt.FinancialPaymentIntentID != nil &&
		*attempt.FinancialPaymentIntentID != input.PaymentIntentID {
		return paymentIntentMismatchConflictError()
	}

	return nil
}

func duplicateSuccessIdentityMatches(
	attempt model.PaymentAttempt,
	input ProcessInput,
) bool {
	if input.PaymentIntentID == "" {
		return false
	}

	if attempt.FinancialPaymentIntentID == nil ||
		*attempt.FinancialPaymentIntentID != input.PaymentIntentID {
		return false
	}

	if input.PaymentAttemptID != "" &&
		attempt.FinancialPaymentAttemptID != nil &&
		*attempt.FinancialPaymentAttemptID != input.PaymentAttemptID {
		return false
	}

	if input.PaidAmountMinor != attempt.AmountMinor {
		return false
	}

	if input.Currency != attempt.Currency {
		return false
	}

	if !input.InstallmentID.IsZero() && attempt.InstallmentID != input.InstallmentID {
		return false
	}

	return true
}

func validatePaymentAttemptSnapshot(
	attempt model.PaymentAttempt,
	installmentRow model.Installment,
	charges []model.InstallmentCharge,
	payableAmount uint64,
) (model.PaymentReconciliationReason, bool) {
	if attempt.AmountMinor != payableAmount {
		return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
	}

	if len(attempt.Items) == 0 {
		return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
	}

	chargeByID := make(map[uuid.UUID]model.InstallmentCharge, len(charges))
	for _, charge := range charges {
		chargeByID[charge.ID.UUID()] = charge
	}

	principalCount := 0
	upfrontCount := 0
	seenChargeSources := make(map[uuid.UUID]struct{})
	calculatedTotal, totalOK := model.PayableAmountFromItems(attempt.Items)
	if !totalOK || calculatedTotal != attempt.AmountMinor {
		return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
	}

	for _, item := range attempt.Items {
		if item.AmountMinor == 0 {
			return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
		}

		if !item.PaymentAttemptID.IsZero() && item.PaymentAttemptID != attempt.ID {
			return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
		}

		switch item.ItemType {
		case model.PrincipalPaymentAttemptItemType:
			principalCount++

			if item.SourceID != installmentRow.ID.UUID() {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

			if item.AmountMinor != installmentRow.PrincipalAmountMinor {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

			if item.Effect != model.DebitPaymentAttemptItemEffect {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

		case model.UpfrontPaymentAttemptItemType:
			upfrontCount++

			if item.SourceID != installmentRow.ID.UUID() {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

			if item.AmountMinor != installmentRow.UpfrontAmountMinor {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

			if item.Effect != model.DebitPaymentAttemptItemEffect {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

		case model.ChargePaymentAttemptItemType:
			if _, exists := seenChargeSources[item.SourceID]; exists {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

			seenChargeSources[item.SourceID] = struct{}{}

			charge, ok := chargeByID[item.SourceID]
			if !ok {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

			if charge.InstallmentID != installmentRow.ID {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

			if !model.IsCustomerPayableCharge(charge) {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

			if item.AmountMinor != charge.AmountMinor {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

			expectedEffect, err := charge.ChargeType.CustomerPaymentChargeEffect()
			if err != nil || item.Effect != expectedEffect {
				return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
			}

		default:
			return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
		}
	}

	if principalCount != 1 {
		return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
	}

	expectedUpfrontCount := 0
	if installmentRow.UpfrontAmountMinor > 0 {
		expectedUpfrontCount = 1
	}

	if upfrontCount != expectedUpfrontCount {
		return model.PaymentAttemptSnapshotInconsistentPaymentReconciliationReason, false
	}

	return "", true
}

func reconciliationReasonValue(reason *model.PaymentReconciliationReason) model.PaymentReconciliationReason {
	if reason == nil {
		return model.UnknownPaymentAttemptPaymentReconciliationReason
	}

	return *reason
}
