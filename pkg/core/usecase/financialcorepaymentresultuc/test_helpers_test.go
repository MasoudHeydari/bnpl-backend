package financialcorepaymentresultuc

import (
	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
)

const testPaymentApplicationResultsDestination = "financial_core.payment_application_results"
const testAuditDestination = "bnpl.audit.events.v1"

type testIDGenerator struct {
	value string
}

func (g testIDGenerator) Generate() string {
	if g.value == "" {
		return "test-event-id"
	}

	return g.value
}

var _ identifier.Generator = testIDGenerator{}

func testFingerprintHasher() *sha256fingerprint.Hasher {
	return sha256fingerprint.New()
}
