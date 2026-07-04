//go:build e2e || legacy_e2e

package e2e_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
)

func TestE2E_Health_HealthyApplication(t *testing.T) {
	for _, path := range []string{"/healthz/live", "/healthz/ready"} {
		t.Run(path, func(t *testing.T) {
			response, err := e2eClient.Get(e2eBaseURL + path)
			require.NoError(t, err)
			defer response.Body.Close()
			require.Equal(t, http.StatusOK, response.StatusCode)

			var body struct {
				Status string `json:"status"`
			}
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			assert.Equal(t, "UP", body.Status)
			assertTraceHeadersGenerated(t, response.Header)
		})
	}
}

func TestE2E_Health_TraceHeadersPreserveClientValues(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, e2eBaseURL+"/healthz/ready", nil)
	require.NoError(t, err)

	headers := map[string]string{
		header.TraceID:       "client-trace-id",
		header.RequestID:     "client-request-id",
		header.CorrelationID: "client-correlation-id",
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := e2eClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	for key, value := range headers {
		assert.Equal(t, value, response.Header.Get(key))
	}
}

func assertTraceHeadersGenerated(t *testing.T, headers http.Header) {
	t.Helper()
	assert.NotEmpty(t, headers.Get(header.TraceID))
	assert.NotEmpty(t, headers.Get(header.RequestID))
	assert.NotEmpty(t, headers.Get(header.CorrelationID))
}
