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
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	assert.Equal(t, cerr.CodeValidationFailed.HTTPStatusCode(), w.Code)
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

	assert.Equal(t, cerr.CodeInternalError.HTTPStatusCode(), w.Code)
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

	assert.Equal(t, cerr.CodeInvalidArgument.HTTPStatusCode(), w.Code)
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
			assert.Equal(t, tt.err.Err.Error(), resp.Message)
			assert.Equal(t, "trace-sererr", resp.TraceID)
		})
	}
}

func TestSerErr_EmptyCodeDefaultsToInternalError(t *testing.T) {
	t.Parallel()

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, &cerr.Error{Err: errors.New("no code"), Code: ""})

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, string(cerr.CodeInternalError), resp.Code)
}

func TestSerErr_NilDataReturnsEmptyDetails(t *testing.T) {
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

	details := ErrorDetails{
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

func TestSerErr_ViolationSliceDerivesFields(t *testing.T) {
	t.Parallel()

	violations := []ErrorViolation{
		{Code: "REQUIRED", Message: "name is required", Fields: []string{"name"}},
		{Code: "REQUIRED", Message: "email is required", Fields: []string{"email"}},
	}

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, cerr.ValidationFailed(errors.New("invalid"), violations))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.ElementsMatch(t, []string{"name", "email"}, resp.Details.Fields)
	assert.Len(t, resp.Details.Violations, 2)
}

func TestSerErr_FieldErrorMapConverted(t *testing.T) {
	t.Parallel()

	fieldErrors := map[string][]string{
		"amount": {"must be positive"},
	}

	c, w := newBindContext("", "trace-sererr")
	SerErr(c, cerr.ValidationFailed(errors.New("invalid"), fieldErrors))

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Contains(t, resp.Details.Fields, "amount")
	require.Len(t, resp.Details.Violations, 1)
	assert.Equal(t, "INVALID", resp.Details.Violations[0].Code)
	assert.Equal(t, "must be positive", resp.Details.Violations[0].Message)
	assert.Equal(t, []string{"amount"}, resp.Details.Violations[0].Fields)
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
	SerValidationErr(c, "amount", "must be greater than zero")

	var resp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, cerr.CodeValidationFailed.HTTPStatusCode(), w.Code)
	assert.Equal(t, string(cerr.CodeValidationFailed), resp.Code)
	assert.Equal(t, "The request contains invalid fields.", resp.Message)
	assert.Equal(t, "trace-validation", resp.TraceID)
	assert.Equal(t, []string{"amount"}, resp.Details.Fields)
	require.Len(t, resp.Details.Violations, 1)
	assert.Equal(t, "INVALID", resp.Details.Violations[0].Code)
	assert.Equal(t, "must be greater than zero", resp.Details.Violations[0].Message)
	assert.Equal(t, []string{"amount"}, resp.Details.Violations[0].Fields)
}

func TestAddErr(t *testing.T) {
	t.Parallel()

	var errs map[string][]string
	AddErr(&errs, "name", "required")
	AddErr(&errs, "name", "too short")

	require.Len(t, errs, 1)
	assert.Equal(t, []string{"required", "too short"}, errs["name"])
}

func newBindContext(body, traceID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ctx := context.Background()
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
		assert.NotEmpty(t, v.Message)
		assert.NotEmpty(t, v.Fields)
	}
}

func TestDetailsFromData_UnknownShapeIgnored(t *testing.T) {
	t.Parallel()

	details := detailsFromData(struct{ Secret string }{Secret: "hidden"})
	assert.Empty(t, details.Fields)
	assert.Empty(t, details.Violations)
}

func TestDetailsFromData_PointerDetails(t *testing.T) {
	t.Parallel()

	ptr := &ErrorDetails{
		Fields: []string{"id"},
		Violations: []ErrorViolation{{
			Code:    "INVALID",
			Message: "bad id",
			Fields:  []string{"id"},
		}},
	}

	details := detailsFromData(ptr)
	assert.Equal(t, []string{"id"}, details.Fields)
	require.Len(t, details.Violations, 1)
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
