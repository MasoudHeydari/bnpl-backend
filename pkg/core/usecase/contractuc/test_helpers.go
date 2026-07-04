package contractuc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

type testIDGenerator struct {
	value string
}

// Generate returns the configured test identifier, or a default when empty.
func (g testIDGenerator) Generate() string {
	if g.value == "" {
		return "test-event-id"
	}

	return g.value
}

var _ trace.IDGenerator = testIDGenerator{}

func mustTestCheckoutRegistry(t *testing.T) *checkout.Registry {
	t.Helper()

	registry, err := checkoutcatalog.NewRegistry()
	require.NoError(t, err)

	return registry
}

func mustTestOriginationJurisdictions(t *testing.T) domaincapability.OriginationJurisdictions {
	t.Helper()

	caps, err := domaincapability.NewJurisdictions([]model.Jurisdiction{model.IRJurisdiction})
	require.NoError(t, err)

	return caps
}
