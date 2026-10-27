package collectionsuc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	collectionsv1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"
)

const (
	collectionsOpenFingerprintVersion    = "collections-default-open:v1:"
	collectionsRecoverFingerprintVersion = "collections-default-recover:v1:"
)

func (uc *UseCase) generateFingerprint(canonicalData []byte) (string, error) {
	const op = "collectionsuc.UseCase.generateFingerprint"

	value, err := uc.fingerprintHasher.Hash(string(canonicalData))
	if err != nil {
		return "", fmt.Errorf("%s: hash fingerprint: %w", op, err)
	}

	if err := value.Validate(); err != nil {
		return "", fmt.Errorf("%s: validate fingerprint: %w", op, err)
	}

	return value.String(), nil
}

func (uc *UseCase) openDecisionFingerprint(
	customerID model.CustomerID,
	triggerInstallmentIDs []model.InstallmentID,
) (string, error) {
	ids := make([]string, 0, len(triggerInstallmentIDs))
	for _, id := range triggerInstallmentIDs {
		ids = append(ids, id.String())
	}

	sort.Strings(ids)

	raw := strings.Join([]string{
		collectionsv1.PolicyID,
		collectionsv1.PolicyVersion,
		customerID.String(),
		strings.Join(ids, ","),
	}, "|")

	return uc.generateFingerprint(append([]byte(collectionsOpenFingerprintVersion), raw...))
}

func (uc *UseCase) recoverDecisionFingerprint(
	episodeID model.DefaultEpisodeID,
	priorDecisionID model.DecisionID,
) (string, error) {
	raw := strings.Join([]string{
		collectionsv1.PolicyID,
		collectionsv1.PolicyVersion,
		"recover",
		episodeID.String(),
		priorDecisionID.String(),
	}, "|")

	return uc.generateFingerprint(append([]byte(collectionsRecoverFingerprintVersion), raw...))
}

func paymentUncertaintyStateKey(installmentIDs []model.InstallmentID) string {
	ids := make([]string, 0, len(installmentIDs))
	for _, id := range installmentIDs {
		ids = append(ids, id.String())
	}

	sort.Strings(ids)

	return strings.Join(ids, ",")
}
