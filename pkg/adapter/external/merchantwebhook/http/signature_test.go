package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignCoversVersionIDTimestampAndBody(t *testing.T) {
	t.Parallel()

	secret := []byte("s3cret")
	body := []byte(`{"event_id":"e1"}`)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("v1\ne1\n1700000000\n" + string(body)))
	want := "v1=" + hex.EncodeToString(mac.Sum(nil))

	require.Equal(t, want, Sign(secret, "e1", 1700000000, body))

	base := Sign(secret, "e1", 1700000000, body)
	require.NotEqual(t, base, Sign(secret, "e2", 1700000000, body))
	require.NotEqual(t, base, Sign(secret, "e1", 1700000001, body))
	require.NotEqual(t, base, Sign(secret, "e1", 1700000000, []byte(`{"event_id":"e2"}`)))
	require.NotEqual(t, base, Sign([]byte("other"), "e1", 1700000000, body))
}
