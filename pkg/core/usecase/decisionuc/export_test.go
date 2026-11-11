package decisionuc

import (
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CheckoutFingerprintForTest exposes checkoutFingerprint for external package tests.
func CheckoutFingerprintForTest(
	uc *UseCase,
	checkoutKey string,
	externalSubjectID model.ExternalSubjectID,
	purchaseContext PurchaseContext,
	requestedAmountMinor uint64,
	currency string,
	policyID string,
	policyVersion string,
) (string, error) {
	return uc.checkoutFingerprint(
		checkoutKey,
		externalSubjectID,
		purchaseContext,
		requestedAmountMinor,
		currency,
		policyID,
		policyVersion,
	)
}
