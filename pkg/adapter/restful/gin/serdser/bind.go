package serdser

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// Bind deserializes and validates the request body into req.
//
// Bind returns true when binding succeeds. When binding fails, it writes a
// standardized APIError response to the client and returns false.
//
// Binding failures are classified as:
//
//   - invalid validator configuration: INTERNAL_ERROR / 500
//   - request validation failure: VALIDATION_FAILED / 422
//   - malformed or unsupported request payload: INVALID_ARGUMENT / 400
func Bind(c *gin.Context, req any, b binding.Binding) bool {
	err := c.ShouldBindWith(req, b)
	if err == nil {
		return true
	}

	traceID := trace.TraceID(c.Request.Context())

	if _, ok := errors.AsType[*validator.InvalidValidationError](err); ok {
		c.JSON(
			cerr.CodeInternalError.HTTPStatusCode(),
			ErrorResponse{
				Code:    string(cerr.CodeInternalError),
				Message: "An unexpected error occurred.",
				TraceID: traceID,
				Details: emptyDetails(),
			},
		)
		return false
	}

	if validationErrs, ok := errors.AsType[validator.ValidationErrors](err); ok {
		fields, violations := validationDetails(validationErrs)

		c.JSON(
			cerr.CodeValidationFailed.HTTPStatusCode(),
			ErrorResponse{
				Code:    string(cerr.CodeValidationFailed),
				Message: "The request contains invalid fields.",
				TraceID: traceID,
				Details: ErrorDetails{
					Fields:     fields,
					Violations: violations,
				},
			},
		)
		return false
	}

	c.JSON(cerr.CodeInvalidArgument.HTTPStatusCode(), ErrorResponse{
		Code:    string(cerr.CodeInvalidArgument),
		Message: "The request payload is invalid.",
		TraceID: traceID,
		Details: emptyDetails(),
	})
	return false
}

// SerValidationErr writes a single-field validation error response.
//
// Use this helper when validation is performed manually inside a handler
// instead of through Gin binding tags.
func SerValidationErr(c *gin.Context, fieldName, msg string) {
	traceID := trace.TraceID(c.Request.Context())

	c.JSON(cerr.CodeValidationFailed.HTTPStatusCode(), ErrorResponse{
		Code:    string(cerr.CodeValidationFailed),
		Message: "The request contains invalid fields.",
		TraceID: traceID,
		Details: ErrorDetails{
			Fields: []string{fieldName},
			Violations: []ErrorViolation{{
				Code:    "INVALID",
				Message: msg,
				Fields:  []string{fieldName},
			}},
		},
	})
}

// AddErr appends validation messages to a field-keyed error map.
//
// This helper is useful when a handler needs to collect multiple validation
// errors before serializing them into a single response.
func AddErr(errs *map[string][]string, name string, msgs ...string) {
	if *errs == nil {
		*errs = make(map[string][]string)
	}

	(*errs)[name] = append((*errs)[name], msgs...)
}

// SerErr writes the standard APIError response for err.
//
// When err wraps a cerr.Error, SerErr uses its stable application code and
// derives the HTTP status code from cerr.Code.HTTPStatusCode.
//
// Unknown errors are serialized as INTERNAL_ERROR with a sanitized message.
// Raw internal error messages must not be exposed to API consumers.
func SerErr(c *gin.Context, err error) {
	traceID := trace.TraceID(c.Request.Context())

	if ce, ok := errors.AsType[*cerr.Error](err); ok {
		code := ce.Code
		if code == "" {
			code = cerr.CodeInternalError
		}

		c.JSON(
			code.HTTPStatusCode(),
			ErrorResponse{
				Code:    string(code),
				Message: ce.Err.Error(),
				TraceID: traceID,
				Details: detailsFromData(ce.Data),
			},
		)
		return
	}

	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Code:    string(cerr.CodeInternalError),
		Message: "An unexpected error occurred.",
		TraceID: traceID,
		Details: emptyDetails(),
	})
}

// emptyDetails returns a non-nil Details value.
//
// Returning empty slices instead of nil keeps the JSON response shape stable.
func emptyDetails() ErrorDetails {
	return ErrorDetails{
		Fields:     []string{},
		Violations: []ErrorViolation{},
	}
}

// validationDetails converts validator.ValidationErrors into APIError details.
func validationDetails(errs validator.ValidationErrors) ([]string, []ErrorViolation) {
	fields := make([]string, 0, len(errs))
	violations := make([]ErrorViolation, 0, len(errs))
	seen := make(map[string]struct{}, len(errs))

	for _, ferr := range errs {
		fieldName := ferr.Field()

		if _, ok := seen[fieldName]; !ok {
			seen[fieldName] = struct{}{}
			fields = append(fields, fieldName)
		}

		violations = append(
			violations,
			ErrorViolation{
				Code:    ferr.Tag(),
				Message: ferr.Error(),
				Fields:  []string{fieldName},
			},
		)
	}

	return fields, violations
}

// detailsFromData converts cerr.Error.Data into Details when possible.
//
// Supported data forms:
//
//   - nil: empty details
//   - Details: returned as-is
//   - *Details: dereferenced when non-nil
//   - []Violation: used as Details.Violations
//   - map[string][]string: converted to field violations
//
// Unknown data shapes are intentionally ignored to avoid leaking internal
// structures into the public REST contract.
func detailsFromData(data any) ErrorDetails {
	switch d := data.(type) {
	case nil:
		return emptyDetails()

	case ErrorDetails:
		return normalizeDetails(d)

	case *ErrorDetails:
		if d == nil {
			return emptyDetails()
		}
		return normalizeDetails(*d)

	case []ErrorViolation:
		fields := fieldsFromViolations(d)
		return ErrorDetails{
			Fields:     fields,
			Violations: d,
		}

	case map[string][]string:
		return detailsFromFieldErrors(d)

	default:
		return emptyDetails()
	}
}

// normalizeDetails ensures slices inside Details are non-nil.
func normalizeDetails(d ErrorDetails) ErrorDetails {
	if d.Fields == nil {
		d.Fields = []string{}
	}
	if d.Violations == nil {
		d.Violations = []ErrorViolation{}
	}
	return d
}

// detailsFromFieldErrors converts a field-keyed validation map into Details.
func detailsFromFieldErrors(errs map[string][]string) ErrorDetails {
	fields := make([]string, 0, len(errs))
	violations := make([]ErrorViolation, 0)

	for field, messages := range errs {
		fields = append(fields, field)

		for _, msg := range messages {
			violations = append(
				violations,
				ErrorViolation{
					Code:    "INVALID",
					Message: msg,
					Fields:  []string{field},
				},
			)
		}
	}

	return ErrorDetails{
		Fields:     fields,
		Violations: violations,
	}
}

// fieldsFromViolations returns the unique field names referenced by violations.
func fieldsFromViolations(violations []ErrorViolation) []string {
	seen := make(map[string]struct{})
	fields := make([]string, 0)

	for _, violation := range violations {
		for _, field := range violation.Fields {
			if _, ok := seen[field]; ok {
				continue
			}

			seen[field] = struct{}{}
			fields = append(fields, field)
		}
	}

	return fields
}
