package noop_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/httpclient/noop"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	"github.com/stretchr/testify/require"
)

func TestClient_DoRejectsWithoutNetworkCall(t *testing.T) {
	t.Parallel()

	client, err := noop.New(lognoop.New())
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, "https://example.invalid/path", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.Nil(t, resp)
	require.True(t, errors.Is(err, corehttpclient.ErrDisabled))
	require.NoError(t, client.Close())
}
