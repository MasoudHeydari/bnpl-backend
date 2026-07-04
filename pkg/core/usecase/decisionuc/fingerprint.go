package decisionuc

import (
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// generateFingerprint hashes canonical data and returns a validated persisted fingerprint.
func (uc *UseCase) generateFingerprint(canonicalData []byte) (string, error) {
	const op = "decisionuc.UseCase.generateFingerprint"

	value, err := uc.fingerprintHasher.Hash(string(canonicalData))
	if err != nil {
		return "", fmt.Errorf("%s: hash fingerprint: %w", op, err)
	}

	if err := value.Validate(); err != nil {
		return "", fmt.Errorf("%s: validate fingerprint: %w", op, err)
	}

	return value.String(), nil
}

const checkoutDecisionFingerprintVersion = "checkout-decision:v3:"

// checkoutFingerprint derives a stable hash from material checkout inputs.
//
// Merchant and branch identities are omitted because they are derived from the
// authoritative POS/online source mapping. Channel, source identity and the
// optional merchant-owned order reference are material client inputs and must
// therefore participate in idempotency conflict detection.
func (uc *UseCase) checkoutFingerprint(
	checkoutKey string,
	externalCustomerID model.ExternalCustomerID,
	purchaseContext PurchaseContext,
	requestedAmountMinor uint64,
	currency string,
	policyID string,
	policyVersion string,
) (string, error) {
	raw := strings.Join([]string{
		checkoutKey,
		externalCustomerID.String(),
		string(purchaseContext.Channel),
		purchaseContext.sourceID().String(),
		purchaseContext.MerchantOrderID,
		fmt.Sprintf("%d", requestedAmountMinor),
		currency,
		policyID,
		policyVersion,
	}, "|")

	canonicalData := append([]byte(checkoutDecisionFingerprintVersion), raw...)

	return uc.generateFingerprint(canonicalData)
}
