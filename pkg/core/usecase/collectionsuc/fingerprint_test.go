package collectionsuc

import (
	"regexp"
	"testing"

	"github.com/parstis/bnpl-backend/pkg/core/policy/collections"
	collectionsv1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestOpenAndRecoverFingerprints_MatchDecisionsCheckConstraint(t *testing.T) {
	t.Parallel()

	uc := &UseCase{fingerprintHasher: sha256fingerprint.New(), policies: legacyTestPoliciesForUnit()}
	customerID := model.CustomerID(uuid.New())
	triggerA := model.InstallmentID(uuid.New())
	triggerB := model.InstallmentID(uuid.New())

	openFP, err := uc.openDecisionFingerprint(customerID, []model.InstallmentID{triggerB, triggerA})
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^sha256:[0-9a-f]{64}$`), openFP)

	// Trigger order must not change the digest.
	openFP2, err := uc.openDecisionFingerprint(customerID, []model.InstallmentID{triggerA, triggerB})
	require.NoError(t, err)
	require.Equal(t, openFP, openFP2)

	episodeID := model.DefaultEpisodeID(uuid.New())
	prior := model.DecisionID(uuid.New())
	recoverFP, err := uc.recoverDecisionFingerprint(episodeID, prior)
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^sha256:[0-9a-f]{64}$`), recoverFP)
	require.NotEqual(t, openFP, recoverFP)
}

func legacyTestPoliciesForUnit() *collections.Registry {
	r, _ := collections.NewRegistry(collectionsv1.New())
	return r
}
