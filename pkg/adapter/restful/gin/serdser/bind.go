package serdser

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

	resti18n "github.com/parstis/bnpl-backend/pkg/adapter/restful/i18n"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// Bind deserializes and validates the request body into req.
//
// Bind returns true when binding succeeds. When binding fails, it writes a
// standardized ErrorResponse to the client and returns false.
//
// Binding failures are classified as:
//   - invalid validator configuration: INTERNAL_ERROR / 500
//   - request validation failure: VALIDATION_FAILED / 422
//   - malformed or unsupported request payload: INVALID_ARGUMENT / 400
func Bind(c *gin.Context, req any, b binding.Binding) bool {
	err := c.ShouldBindWith(req, b)
	if err == nil {
		return true
	}

	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		SerErr(
			c,
			cerr.NewPublic(
				cerr.CodePayloadTooLarge,
				cerr.DefaultMessageKey(cerr.CodePayloadTooLarge),
				err,
				cerr.Details{},
			),
		)
		return false
	}

	var invalidValidation *validator.InvalidValidationError
	if errors.As(err, &invalidValidation) {
		SerErr(c, cerr.InternalError(err))
		return false
	}

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		fields, violations := validationDetails(validationErrs)
		SerErr(
			c,
			cerr.NewPublic(
				cerr.CodeValidationFailed,
				cerr.DefaultMessageKey(cerr.CodeValidationFailed),
				err,
				cerr.Details{Fields: fields, Violations: violations},
			),
		)
		return false
	}

	SerErr(
		c,
		cerr.NewPublic(
			cerr.CodeInvalidArgument,
			"validation.request_payload_invalid",
			err,
			cerr.Details{},
		),
	)
	return false
}

// SerValidationErr writes a single-field validation error response.
//
// messageKey is language-neutral. Optional params are passed to the active
// Localizer implementation, allowing this API to work with template-capable
// providers such as go-i18n without changing callers later.
func SerValidationErr(
	c *gin.Context,
	fieldName string,
	messageKey string,
	params ...resti18n.Params,
) {
	var messageParams cerr.MessageParams
	if len(params) > 0 {
		messageParams = cerr.MessageParams(params[0])
	}

	SerErr(
		c,
		cerr.NewPublic(
			cerr.CodeValidationFailed,
			cerr.DefaultMessageKey(cerr.CodeValidationFailed),
			nil,
			cerr.Details{
				Fields: []string{fieldName},
				Violations: []cerr.Violation{
					{
						Code:          "INVALID",
						MessageKey:    cerr.MessageKey(messageKey),
						MessageParams: messageParams,
						Fields:        []string{fieldName},
					},
				},
			},
		),
	)
}

// SerErr serializes err into the standard public API error response.
//
// Only semantic public fields of cerr.Error (Code, MessageKey, and Details) may
// cross the HTTP boundary. The underlying cause is registered on the Gin
// context for observability purposes, but is never serialized.
//
// Unclassified errors, malformed cerr.Errors, and unknown error codes fail
// closed as INTERNAL_ERROR.
func SerErr(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()

	traceID := trace.ID(c.Request.Context())

	ce, ok := errors.AsType[*cerr.Error](err)
	if !ok || ce.Code == "" {
		writeInternalError(c, traceID)
		return
	}

	code := ce.Code
	statusCode := StatusCode(code)
	if statusCode == http.StatusInternalServerError && code != cerr.CodeInternalError {
		writeInternalError(c, traceID)
		return
	}

	messageKey := ce.MessageKey
	if messageKey == "" {
		messageKey = cerr.DefaultMessageKey(code)
	}

	details := localizedDetails(c, ce.Details)
	if code == cerr.CodeInternalError {
		messageKey = cerr.DefaultMessageKey(cerr.CodeInternalError)
		details = emptyDetails()
	}

	c.JSON(
		statusCode,
		ErrorResponse{
			Code:    string(code),
			Message: localizeError(c.Request.Context(), string(messageKey), resti18n.Params(ce.MessageParams)),
			TraceID: traceID,
			Details: details,
		},
	)
}

func writeInternalError(c *gin.Context, traceID string) {
	c.JSON(
		http.StatusInternalServerError,
		ErrorResponse{
			Code: string(cerr.CodeInternalError),
			Message: localizeError(
				c.Request.Context(),
				string(cerr.DefaultMessageKey(cerr.CodeInternalError)),
				nil,
			),
			TraceID: traceID,
			Details: emptyDetails(),
		},
	)
}

func emptyDetails() *ErrorDetails {
	return &ErrorDetails{
		Fields:     []string{},
		Violations: []ErrorViolation{},
	}
}

func localizedDetails(c *gin.Context, details cerr.Details) *ErrorDetails {
	fields := details.Fields
	if fields == nil {
		fields = []string{}
	}

	violations := make([]ErrorViolation, 0, len(details.Violations))
	for _, violation := range details.Violations {
		fieldsForViolation := violation.Fields
		if fieldsForViolation == nil {
			fieldsForViolation = []string{}
		}

		key := violation.MessageKey
		if key == "" {
			key = "validation.invalid"
		}

		violations = append(violations, ErrorViolation{
			Code: violation.Code,
			Message: localizeError(
				c.Request.Context(),
				string(key),
				resti18n.Params(violation.MessageParams),
			),
			Fields: fieldsForViolation,
		})
	}

	return &ErrorDetails{
		Fields:            fields,
		Violations:        violations,
		RetryAfterSeconds: details.RetryAfterSeconds,
	}
}

func validationDetails(errs validator.ValidationErrors) ([]string, []cerr.Violation) {
	fields := make([]string, 0, len(errs))
	violations := make([]cerr.Violation, 0, len(errs))
	seen := make(map[string]struct{}, len(errs))

	for _, ferr := range errs {
		fieldName := ferr.Field()
		if _, ok := seen[fieldName]; !ok {
			seen[fieldName] = struct{}{}
			fields = append(fields, fieldName)
		}

		violations = append(violations, cerr.Violation{
			Code:       ferr.Tag(),
			MessageKey: cerr.MessageKey(validationMessageKey(ferr.Tag())),
			Fields:     []string{fieldName},
		})
	}

	return fields, violations
}

func validationMessageKey(tag string) string {
	switch tag {
	case "required":
		return "validation.required"

	case "uuid", "uuid3", "uuid4", "uuid5":
		return "validation.uuid"

	case "min":
		return "validation.min"

	case "max":
		return "validation.max"

	case "oneof":
		return "validation.oneof"

	default:
		return "validation.invalid"
	}
}
