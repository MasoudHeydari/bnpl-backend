package merchantterminaluc

import (
	"fmt"
	"strconv"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// fingerprintVersion prefixes every request fingerprint so the canonical form
// can evolve without colliding with fingerprints already stored.
const fingerprintVersion = "v1"

// provisionFingerprint returns the canonical fingerprint of a Provision
// request. Provision carries no caller-chosen parameters besides the merchant,
// so the fingerprint is the merchant identity alone.
func provisionFingerprint(merchantID model.MerchantID) string {
	return fmt.Sprintf("%s|provision|%s", fingerprintVersion, merchantID.UUID())
}

// rotateFingerprint returns the canonical fingerprint of a Rotate request.
//
// effectiveGrace must be the grace period after the default was applied, so a
// request that omits the grace period and one that spells out the default are
// the same request. The grace is rendered as integer nanoseconds to avoid any
// formatting ambiguity.
func rotateFingerprint(merchantID model.MerchantID, effectiveGrace time.Duration) string {
	return fmt.Sprintf(
		"%s|rotate|%s|%s",
		fingerprintVersion,
		merchantID.UUID(),
		strconv.FormatInt(effectiveGrace.Nanoseconds(), 10),
	)
}
