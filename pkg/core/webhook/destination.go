package webhook

import (
	"fmt"
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// DestinationPrefix is the integration_outbox destination scheme for HTTP
// webhook delivery. The dispatcher resolves the current merchant webhook
// configuration at delivery time; URLs and secrets are never snapshotted into
// the outbox row or payload.
const DestinationPrefix = "webhook://merchant/"

// MerchantDestination returns the logical outbox destination for merchantID.
func MerchantDestination(merchantID model.MerchantID) string {
	return DestinationPrefix + merchantID.String()
}

// IsDestination reports whether destination is routed to the webhook dispatcher.
func IsDestination(destination string) bool {
	return strings.HasPrefix(destination, DestinationPrefix)
}

// ParseMerchantDestination extracts the merchant id from a logical webhook
// outbox destination.
func ParseMerchantDestination(destination string) (model.MerchantID, error) {
	const op = "webhook.ParseMerchantDestination"

	raw, ok := strings.CutPrefix(destination, DestinationPrefix)
	if !ok {
		return model.MerchantID{}, fmt.Errorf("%s: destination is not a merchant webhook destination", op)
	}

	merchantID, err := model.ParseMerchantID(raw)
	if err != nil {
		return model.MerchantID{}, fmt.Errorf("%s: parse merchant id: %w", op, err)
	}

	if merchantID.IsZero() {
		return model.MerchantID{}, fmt.Errorf("%s: merchant id is zero", op)
	}

	return merchantID, nil
}
