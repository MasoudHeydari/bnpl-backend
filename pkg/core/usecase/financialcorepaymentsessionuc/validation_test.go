package financialcorepaymentsessionuc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePaymentSessionURL_RequiresHTTPS(t *testing.T) {
	t.Parallel()

	require.NoError(t, validatePaymentSessionURL("https://financial.example/pay/session-1"))
	require.Error(t, validatePaymentSessionURL("http://financial.example/pay/session-1"))
	require.Error(t, validatePaymentSessionURL("/pay/session-1"))
	require.Error(t, validatePaymentSessionURL("https://user:pass@financial.example/pay/session-1"))
}
