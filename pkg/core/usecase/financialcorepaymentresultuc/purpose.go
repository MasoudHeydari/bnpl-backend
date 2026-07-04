package financialcorepaymentresultuc

import (
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const financialCoreContractSourceType = "bnpl_contract"

// resolvePaymentPurpose maps inbound event metadata to a supported payment purpose.
//
// Temporary compatibility: when payment_purpose is empty and source_type identifies a BNPL
// contract checkout source, treat the event as checkout first-installment activation.
func resolvePaymentPurpose(input ProcessInput) model.FinancialCorePaymentPurpose {
	if input.PaymentPurpose != "" {
		return input.PaymentPurpose
	}

	sourceType := strings.TrimSpace(input.SourceType)
	if strings.EqualFold(sourceType, financialCoreContractSourceType) {
		return model.CheckoutFirstInstallmentFinancialCorePaymentPurpose
	}

	if strings.EqualFold(sourceType, financialCorePaymentAttemptSourceType) {
		return model.InstallmentRepaymentFinancialCorePaymentPurpose
	}

	return ""
}

func resolveInstallmentRepaymentTargetReason(input ProcessInput) model.PaymentReconciliationReason {
	if input.ContractID.IsZero() {
		return model.ContractNotFoundPaymentReconciliationReason
	}

	if input.InstallmentID.IsZero() {
		return model.InstallmentNotFoundPaymentReconciliationReason
	}

	return ""
}

func inboxSubjectForInstallmentRepayment(input ProcessInput) (subjectType *string, subjectID *string) {
	if !input.InstallmentID.IsZero() {
		st := string(model.InstallmentInboxSubjectType)
		sid := input.InstallmentID.String()

		return &st, &sid
	}

	if input.PaymentIntentID != "" {
		st := string(model.FinancialPaymentIntentInboxSubjectType)
		sid := input.PaymentIntentID

		return &st, &sid
	}

	return nil, nil
}
