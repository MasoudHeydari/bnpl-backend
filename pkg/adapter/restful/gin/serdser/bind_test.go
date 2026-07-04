package serdser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

type bindSuccessRequest struct {
	Name string `json:"name" binding:"required"`
}

type bindValidationRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type invalidConfigBinding struct{}

func (invalidConfigBinding) Name() string { return "invalid-config" }

func (invalidConfigBinding) Bind(_ *http.Request, _ any) error {
	return &validator.InvalidValidationError{}
}

func TestBind_Success(t *testing.T) {
	t.Parallel()

	c, w := newBindContext(`{"name":"alice"}`, "trace-bind")

	var req bindSuccessRequest
	ok := Bind(c, &req, binding.JSON)

	require.True(t, ok)
	assert.Equal(t, "alice", req.Name)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Body.String())
}

func TestBind_ValidationErrors(t *testing.T) {
	t.Parallel()

	c, w := newBindContext(`{"email":"not-an-email"}`, "trace-bind")

	var req bindValidationRequest
	ok := Bind(c, &req, binding.JSON)

	require.False(t, ok)

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, StatusCode(cerr.CodeValidationFailed), w.Code)
	assert.Equal(t, string(cerr.CodeValidationFailed), resp.Code)
	assert.Equal(t, "The request contains invalid fields.", resp.Message)
	assert.Equal(t, "trace-bind", resp.TraceID)
	require.NotEmpty(t, resp.Details.Fields)
	assert.Contains(t, resp.Details.Fields, "Email")
	require.NotEmpty(t, resp.Details.Violations)
	assert.Equal(t, "email", resp.Details.Violations[0].Code)
}

func TestBind_InvalidValidationError(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-bind")

	var req bindSuccessRequest
	ok := Bind(c, &req, invalidConfigBinding{})

	require.False(t, ok)

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, StatusCode(cerr.CodeInternalError), w.Code)
	assert.Equal(t, string(cerr.CodeInternalError), resp.Code)
	assert.Equal(t, "An unexpected error occurred.", resp.Message)
	assert.Equal(t, "trace-bind", resp.TraceID)
	assert.Empty(t, resp.Details.Fields)
	assert.Empty(t, resp.Details.Violations)
}

func TestBind_InvalidArgument(t *testing.T) {
	t.Parallel()

	c, w := newBindContext(`{"email":`, "trace-bind")

	var req bindValidationRequest
	ok := Bind(c, &req, binding.JSON)

	require.False(t, ok)

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, StatusCode(cerr.CodeInvalidArgument), w.Code)
	assert.Equal(t, string(cerr.CodeInvalidArgument), resp.Code)
	assert.Equal(t, "The request payload is invalid.", resp.Message)
	assert.Equal(t, "trace-bind", resp.TraceID)
	assert.Empty(t, resp.Details.Fields)
	assert.Empty(t, resp.Details.Violations)
}

func TestSerErr_CerrCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        *cerr.Error
		wantStatus int
		wantCode   string
	}{
		{"ValidationFailed", cerr.ValidationFailed(errors.New("bad input")), http.StatusUnprocessableEntity, string(cerr.CodeValidationFailed)},
		{"InvalidArgument", cerr.InvalidArgument(errors.New("bad arg")), http.StatusBadRequest, string(cerr.CodeInvalidArgument)},
		{"ResourceNotFound", cerr.ResourceNotFound(errors.New("missing")), http.StatusNotFound, string(cerr.CodeResourceNotFound)},
		{"DecisionNotFound", cerr.DecisionNotFound(errors.New("missing")), http.StatusNotFound, string(cerr.CodeDecisionNotFound)},
		{"DecisionRejected", cerr.DecisionRejected(errors.New("rejected")), http.StatusUnprocessableEntity, string(cerr.CodeDecisionRejected)},
		{"DecisionExpired", cerr.DecisionExpired(errors.New("expired")), http.StatusUnprocessableEntity, string(cerr.CodeDecisionExpired)},
		{"ContractNotFound", cerr.ContractNotFound(errors.New("missing")), http.StatusNotFound, string(cerr.CodeContractNotFound)},
		{"ContractAlreadyExists", cerr.ContractAlreadyExists(errors.New("exists")), http.StatusConflict, string(cerr.CodeContractAlreadyExists)},
		{"InstallmentNotFound", cerr.InstallmentNotFound(errors.New("missing")), http.StatusNotFound, string(cerr.CodeInstallmentNotFound)},
		{"IdempotencyConflict", cerr.IdempotencyConflict(errors.New("conflict")), http.StatusConflict, string(cerr.CodeIdempotencyConflict)},
		{"PaymentProviderError", cerr.PaymentProviderError(errors.New("provider down")), http.StatusBadGateway, string(cerr.CodePaymentProviderError)},
		{"Conflict", cerr.Conflict(errors.New("conflict")), http.StatusConflict, string(cerr.CodeConflict)},
		{"ServiceUnavailable", cerr.ServiceUnavailable(errors.New("unavailable")), http.StatusServiceUnavailable, string(cerr.CodeServiceUnavailable)},
		{"InternalError", cerr.InternalError(errors.New("boom")), http.StatusInternalServerError, string(cerr.CodeInternalError)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, w := newBindContext("", "trace-sererr")
			SerErr(c, tt.err)

			var resp ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, resp.Code)
			if tt.wantCode == string(cerr.CodeInternalError) {
				assert.Equal(t, "An unexpected error occurred.", resp.Message)
			} else {
				assert.Equal(t, localizedMessageForTest(tt.err), resp.Message)
			}

			assert.Equal(t, "trace-sererr", resp.TraceID)
		})
	}
}

func TestSerErr_EmptyCodeDefaultsToInternalError(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, cerr.NewPublic("", "error.default.internal", errors.New("no code"), cerr.Details{}))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, string(cerr.CodeInternalError), resp.Code)
}

func TestSerErr_ZeroDetailsReturnsEmptyDetails(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, cerr.InternalError(errors.New("boom")))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.NotNil(t, resp.Details.Fields)
	require.NotNil(t, resp.Details.Violations)
	assert.Empty(t, resp.Details.Fields)
	assert.Empty(t, resp.Details.Violations)
}

func TestSerErr_ErrorDetailsNormalized(t *testing.T) {
	t.Parallel()

	details := cerr.Details{
		Fields:     nil,
		Violations: nil,
	}

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, cerr.ValidationFailed(errors.New("invalid"), details))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.NotNil(t, resp.Details.Fields)
	require.NotNil(t, resp.Details.Violations)
	assert.Empty(t, resp.Details.Fields)
	assert.Empty(t, resp.Details.Violations)
}

func TestSerErr_PublicDetailsAreSerialized(t *testing.T) {
	t.Parallel()

	details := cerr.Details{
		Fields: []string{"merchant_id"},
		Violations: []cerr.Violation{{
			Code:       "REQUIRED",
			MessageKey: "error.merchant_id_required",
			Fields:     []string{"merchant_id"},
		}},
	}

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.merchant_id_required",
		errors.New("internal validation cause"),
		details,
	))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "Merchant id is required.", resp.Message)
	assert.Equal(t, []string{"merchant_id"}, resp.Details.Fields)
	require.Len(t, resp.Details.Violations, 1)
	assert.Equal(t, "REQUIRED", resp.Details.Violations[0].Code)
}

func TestSerErr_LocalizesPersian(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-fa")
	c.Request = c.Request.WithContext(withTestLocalization(c.Request.Context(), resti18n.LocaleFAIR))

	SerErr(c, cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.merchant_id_required",
		nil,
		cerr.Details{},
	))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "شناسه پذیرنده الزامی است.", resp.Message)
}

func TestSerErr_UnknownError(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, errors.New("database exploded"))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, string(cerr.CodeInternalError), resp.Code)
	assert.Equal(t, "An unexpected error occurred.", resp.Message)
	assert.Equal(t, "trace-sererr", resp.TraceID)
	assert.Empty(t, resp.Details.Fields)
	assert.Empty(t, resp.Details.Violations)
}

func TestSerValidationErr(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-validation")
	SerValidationErr(c, "amount", "validation.positive_integer", map[string]any{"field": "amount"})

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, StatusCode(cerr.CodeValidationFailed), w.Code)
	assert.Equal(t, string(cerr.CodeValidationFailed), resp.Code)
	assert.Equal(t, "The request contains invalid fields.", resp.Message)
	assert.Equal(t, "trace-validation", resp.TraceID)
	assert.Equal(t, []string{"amount"}, resp.Details.Fields)
	require.Len(t, resp.Details.Violations, 1)
	assert.Equal(t, "INVALID", resp.Details.Violations[0].Code)
	assert.Equal(t, "amount must be a positive integer.", resp.Details.Violations[0].Message)
	assert.Equal(t, []string{"amount"}, resp.Details.Violations[0].Fields)
}

func newBindContext(body, traceID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ctx := withTestLocalization(context.Background(), resti18n.LocaleENUS)
	ctx = trace.WithTraceID(ctx, traceID)

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(http.MethodPost, "/", nil)
	} else {
		req = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	}

	req = req.WithContext(ctx)
	c.Request = req

	return c, w
}

func TestValidationDetails(t *testing.T) {
	t.Parallel()

	validate := validator.New()
	err := validate.Struct(struct {
		Email string `validate:"required,email"`
		Name  string `validate:"required"`
	}{
		Email: "bad",
	})

	var validationErrs validator.ValidationErrors
	require.True(t, errors.As(err, &validationErrs))

	fields, violations := validationDetails(validationErrs)

	assert.Contains(t, fields, "Email")
	assert.Contains(t, fields, "Name")
	assert.Len(t, violations, 2)
	for _, v := range violations {
		assert.NotEmpty(t, v.Code)
		assert.NotEmpty(t, v.MessageKey)
		assert.NotEmpty(t, v.Fields)
	}
}

func TestSerErr_MessageDoesNotLeakInternalDetails(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, errors.New("SELECT * FROM users WHERE password='secret'"))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.False(t, strings.Contains(resp.Message, "SELECT"))
	assert.Equal(t, "An unexpected error occurred.", resp.Message)
}

func TestSerErr_ClientErrorDoesNotLeakCause(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, cerr.ValidationFailed(errors.New(`ERROR: duplicate key violates constraint "secret_constraint"`)))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.Equal(t, "The request contains invalid fields.", resp.Message)
	assert.NotContains(t, resp.Message, "secret_constraint")
}

func TestSerErr_ServiceUnavailableDoesNotLeakCause(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, cerr.ServiceUnavailable(errors.New("dial tcp financial-core.prod.svc: connection refused")))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, "A required service is temporarily unavailable.", resp.Message)
	assert.NotContains(t, resp.Message, "financial-core.prod.svc")
}

func TestSerErr_InternalErrorCerrDoesNotLeakCause(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, cerr.NewPublic(
		cerr.CodeInternalError,
		"error.default.internal",
		errors.New(`ERROR: column "display_name" does not exist (SQLSTATE 42703)`),
		cerr.Details{
			Fields: []string{"internal_column"},
			Violations: []cerr.Violation{{
				Code:       "DB_SCHEMA_ERROR",
				MessageKey: "error.default.internal",
				Fields:     []string{"internal_column"},
			}},
		},
	))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, string(cerr.CodeInternalError), resp.Code)
	assert.Equal(t, "An unexpected error occurred.", resp.Message)
	assert.NotContains(t, resp.Message, "display_name")
	assert.NotContains(t, resp.Message, "SQLSTATE")
	require.NotNil(t, resp.Details)
	assert.Empty(t, resp.Details.Fields)
	assert.Empty(t, resp.Details.Violations)
}

func TestSerErr_DoesNotLeakFinancialCorePaymentTerms(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-sererr-fc")
	SerErr(c, cerr.InternalError(errors.New(
		"submit payment: financial_payment_intent_id=fpi_01HXYZ payment_session_id=fps_01HXYZ invoice_id=inv_01HXYZ provider_code=default",
	)))

	body := w.Body.String()
	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, string(cerr.CodeInternalError), resp.Code)
	for _, term := range []string{
		"financial_payment_",
		"payment_intent_id",
		"payment_session_id",
		"provider_code",
		"invoice_id",
		"fpi_01HXYZ",
		"fps_01HXYZ",
	} {
		assert.NotContains(t, body, term)
	}
}
