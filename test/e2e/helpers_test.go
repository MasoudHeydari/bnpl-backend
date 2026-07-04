//go:build e2e || legacy_e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/serdser"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

// authHeaders builds the trusted gateway authentication headers consumed by
// middleware.AuthContext (pkg/adapter/restful/gin/middleware/auth_context.go).
// Tenant and context IDs are freshly generated UUIDs for each call.
func authHeaders(subjectID model.SubjectID, permissions ...string) map[string]string {
	return map[string]string{
		header.AuthSubjectID:   subjectID.String(),
		header.AuthTenantID:    uuid.NewString(),
		header.AuthContextID:   uuid.NewString(),
		header.AuthPermissions: strings.Join(permissions, ","),
	}
}

// creditCreateAuthHeaders returns auth headers for the initial credit-limit
// endpoint (permission bnpl:credit-limits:create:self). SubjectID is the customer
// identity ID (customer_profiles.identity_subject_id).
func creditCreateAuthHeaders(identitySubjectID model.IdentitySubjectID) map[string]string {
	return authHeaders(model.SubjectID(identitySubjectID), permission.CreditLimitsCreateSelf.Code)
}

// withoutHeader returns a copy of headers with name removed.
func withoutHeader(headers map[string]string, name string) map[string]string {
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		if k == name {
			continue
		}

		out[k] = v
	}
	return out
}

// withHeader returns a copy of headers with name set to value.
func withHeader(headers map[string]string, name, value string) map[string]string {
	out := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		out[k] = v
	}
	out[name] = value
	return out
}

// doRequest issues method against path with the given headers and raw body.
//
// body may be nil to send no request body at all, which is distinct from an
// empty byte slice (an empty, zero-length body).
func doRequest(t *testing.T, method, path string, headers map[string]string, body []byte) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	request, err := http.NewRequest(method, e2eBaseURL+path, reader)
	require.NoError(t, err)

	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	for name, value := range headers {
		request.Header.Set(name, value)
	}

	response, err := e2eClient.Do(request)
	require.NoError(t, err)

	return response
}

// postJSON marshals payload and issues an HTTP POST to path with headers.
func postJSON(t *testing.T, path string, headers map[string]string, payload any) *http.Response {
	t.Helper()

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	return doRequest(t, http.MethodPost, path, headers, body)
}

// successEnvelope mirrors serdser.SuccessResponse with a typed Data payload so
// tests can decode endpoint-specific response bodies without depending on
// unexported production types.
type successEnvelope[T any] struct {
	Data T                   `json:"data"`
	Meta serdser.SuccessMeta `json:"meta"`
}

// decodeSuccess decodes a platform success envelope and closes the response body.
func decodeSuccess[T any](t *testing.T, response *http.Response) (T, serdser.SuccessMeta) {
	t.Helper()
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	var envelope successEnvelope[T]
	require.NoErrorf(t, json.Unmarshal(raw, &envelope), "decode success envelope: %s", string(raw))

	return envelope.Data, envelope.Meta
}

// decodeError decodes a platform error envelope and closes the response body.
func decodeError(t *testing.T, response *http.Response) serdser.ErrorResponse {
	t.Helper()
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	var errResp serdser.ErrorResponse
	require.NoErrorf(t, json.Unmarshal(raw, &errResp), "decode error envelope: %s", string(raw))

	return errResp
}

// drainAndClose discards and closes response, for calls where only the status
// code matters. Safe to call from any goroutine: it never invokes testify.
func drainAndClose(response *http.Response) {
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		_ = response.Body.Close()
		return
	}

	_ = response.Body.Close()
}

// readAndClose reads response fully into memory and closes it, returning the
// raw bytes for later decoding. Safe to call from any goroutine: like
// drainAndClose, it never invokes testify, so goroutines that collect
// concurrent HTTP responses can decode and assert on them afterward from the
// main test goroutine.
func readAndClose(response *http.Response) ([]byte, error) {
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}

// assertValidUUID parses value as a UUID, failing the test with diagnostic
// context when it is missing, malformed, or the nil UUID.
func assertValidUUID(t *testing.T, value string, msgAndArgs ...any) uuid.UUID {
	t.Helper()

	parsed, err := uuid.Parse(value)
	require.NoErrorf(t, err, "expected a valid UUID, got %q: %v", value, msgAndArgs)
	assert.NotEqualf(t, uuid.Nil, parsed, "expected a non-nil UUID: %v", msgAndArgs)

	return parsed
}

// assertNonZeroTime fails the test when tm is the zero time.
func assertNonZeroTime(t *testing.T, tm time.Time, msgAndArgs ...any) {
	t.Helper()
	assert.Falsef(t, tm.IsZero(), "expected a non-zero timestamp: %v", msgAndArgs)
}
