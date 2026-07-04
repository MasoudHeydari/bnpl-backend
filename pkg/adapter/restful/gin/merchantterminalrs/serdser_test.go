package merchantterminalrs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/permission"
)

func newTestContext(
	t *testing.T,
	method string,
	target string,
	body string,
	authCtx *permission.AuthContext,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	if authCtx != nil {
		req = req.WithContext(permission.WithAuthContext(req.Context(), *authCtx))
	}

	c.Request = req

	return c, recorder
}

func testAuthContext() permission.AuthContext {
	return permission.AuthContext{
		SubjectID: model.SubjectID(uuid.New()),
	}
}

func TestToRotateInput_EmptyBodyUsesDefaultGrace(t *testing.T) {
	t.Parallel()

	authCtx := testAuthContext()
	c, _ := newTestContext(
		t,
		http.MethodPost,
		"/rotate",
		"",
		&authCtx,
	)
	c.Request.Header.Set(header.IdempotencyKey, "  key-1 ")

	input, ok := toRotateInput(c)

	require.True(t, ok)
	require.Equal(t, authCtx.SubjectID.AsMerchantID(), input.MerchantID)
	require.Equal(t, "key-1", input.IdempotencyKey)
	require.Nil(t, input.GracePeriod)
}

func TestToRotateInput_GracePeriodSeconds(t *testing.T) {
	t.Parallel()

	authCtx := testAuthContext()
	c, _ := newTestContext(
		t,
		http.MethodPost,
		"/rotate",
		`{"grace_period_seconds":3600}`,
		&authCtx,
	)
	c.Request.Header.Set(header.IdempotencyKey, "key-1")

	input, ok := toRotateInput(c)

	require.True(t, ok)
	require.NotNil(t, input.GracePeriod)
	require.Equal(t, time.Hour, *input.GracePeriod)
}

func TestToRotateInput_HugeGraceIsClampedWithoutOverflow(t *testing.T) {
	t.Parallel()

	authCtx := testAuthContext()
	c, _ := newTestContext(
		t,
		http.MethodPost,
		"/rotate",
		`{"grace_period_seconds":9223372036854775807}`,
		&authCtx,
	)
	c.Request.Header.Set(header.IdempotencyKey, "key-1")

	input, ok := toRotateInput(c)

	require.True(t, ok)
	require.NotNil(t, input.GracePeriod)
	require.Positive(t, int64(*input.GracePeriod))
}

func TestToRotateInput_RejectsNegativeGrace(t *testing.T) {
	t.Parallel()

	authCtx := testAuthContext()
	c, recorder := newTestContext(
		t,
		http.MethodPost,
		"/rotate",
		`{"grace_period_seconds":-1}`,
		&authCtx,
	)
	c.Request.Header.Set(header.IdempotencyKey, "key-1")

	_, ok := toRotateInput(c)

	require.False(t, ok)
	require.True(t, c.IsAborted())
	require.NotEqual(t, http.StatusOK, recorder.Code)
}

func TestToRotateInput_RequiresIdempotencyKey(t *testing.T) {
	t.Parallel()

	authCtx := testAuthContext()
	c, _ := newTestContext(
		t,
		http.MethodPost,
		"/rotate",
		"",
		&authCtx,
	)

	_, ok := toRotateInput(c)

	require.False(t, ok)
	require.True(t, c.IsAborted())
}

func TestToRotateInput_RequiresAuthContext(t *testing.T) {
	t.Parallel()

	c, _ := newTestContext(
		t,
		http.MethodPost,
		"/rotate",
		"",
		nil,
	)
	c.Request.Header.Set(header.IdempotencyKey, "key-1")

	_, ok := toRotateInput(c)

	require.False(t, ok)
	require.True(t, c.IsAborted())
}

func TestToRetireInput(t *testing.T) {
	t.Parallel()

	authCtx := testAuthContext()
	credentialID := uuid.New()

	c, _ := newTestContext(
		t,
		http.MethodPost,
		"/retire",
		"",
		&authCtx,
	)
	c.Params = gin.Params{{Key: "credential_id", Value: credentialID.String()}}

	input, ok := toRetireInput(c)

	require.True(t, ok)
	require.Equal(t, authCtx.SubjectID.AsMerchantID(), input.MerchantID)
	require.Equal(t, credentialID, input.CredentialID.UUID())

	bad, _ := newTestContext(
		t,
		http.MethodPost,
		"/retire",
		"",
		&authCtx,
	)
	bad.Params = gin.Params{{Key: "credential_id", Value: "nope"}}

	_, ok = toRetireInput(bad)

	require.False(t, ok)
	require.True(t, bad.IsAborted())
}

func TestToProvisionInput(t *testing.T) {
	t.Parallel()

	merchantID := uuid.New()
	authCtx := permission.AuthContext{
		SubjectID: model.SubjectID(merchantID),
	}

	c, _ := newTestContext(
		t,
		http.MethodPost,
		"/merchant/terminal",
		"",
		&authCtx,
	)
	c.Request.Header.Set(header.IdempotencyKey, "key-1")

	input, ok := toProvisionInput(c)

	require.True(t, ok)
	require.Equal(t, merchantID, input.MerchantID.UUID())
	require.Equal(t, "key-1", input.IdempotencyKey)

	missingAuth, _ := newTestContext(
		t,
		http.MethodPost,
		"/merchant/terminal",
		"",
		nil,
	)
	missingAuth.Request.Header.Set(header.IdempotencyKey, "key-1")

	_, ok = toProvisionInput(missingAuth)

	require.False(t, ok)
	require.True(t, missingAuth.IsAborted())

	missingKey, _ := newTestContext(
		t,
		http.MethodPost,
		"/merchant/terminal",
		"",
		&authCtx,
	)

	_, ok = toProvisionInput(missingKey)

	require.False(t, ok)
	require.True(t, missingKey.IsAborted())
}
