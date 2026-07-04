package checkoutsessionuc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	fingerprinthmac "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/hmac"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
)

const (
	testOTPTemplateID = "bnpl.test_otp.v1"
)

func newTestOTPHasher(t *testing.T) fingerprint.Hasher {
	t.Helper()

	hasher, err := fingerprinthmac.New([]byte(strings.Repeat("p", 32)))
	require.NoError(t, err)

	return hasher
}
