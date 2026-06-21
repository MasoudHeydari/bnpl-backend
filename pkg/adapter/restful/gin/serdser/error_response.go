package serdser

// ErrorResponse is the platform-wide error response contract.
//
// Every non-2xx REST response must use this structure.
//
// Code is the stable machine-readable error identifier. For application errors,
// it is derived from cerr.Error.Code. HTTP status codes are derived from
// cerr.Code.HTTPStatusCode() by the serializer.
//
// Message is human-readable and intended for clients, logs, and debugging.
// It may change over time and must not be used as a stable client contract.
//
// TraceID lets clients and operators correlate the response with server-side
// logs and traces.
//
// Details carries optional machine-parseable context for validation errors,
// retry hints, and other safe-to-expose metadata.
type ErrorResponse struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	TraceID string       `json:"trace_id"`
	Details ErrorDetails `json:"details"`
}

// ErrorDetails carries machine-parseable context for an error response.
//
// ErrorDetails must only contain information that is safe to expose to API
// clients. It must never include secrets, access tokens, SQL statements,
// stack traces, or internal infrastructure diagnostics.
type ErrorDetails struct {
	// Fields contains the unique request fields related to the error.
	//
	// For non-validation errors, Fields should be an empty slice rather than nil
	// to keep the response shape stable.
	Fields []string `json:"fields"`

	// Violations contains field-level or rule-level validation failures.
	//
	// For non-validation errors, Violations should be an empty slice rather than
	// nil to keep the response shape stable.
	Violations []ErrorViolation `json:"violations"`

	// RetryAfterSeconds optionally tells the client when retrying may be useful.
	//
	// It should be set only for errors where retry semantics are explicit, such
	// as rate limiting, temporary provider failures, or service unavailability.
	RetryAfterSeconds *int `json:"retry_after_seconds,omitempty"`
}

// ErrorViolation describes a single validation or business-rule failure.
//
// A violation may be tied to one or more request fields. For rule-level
// violations that are not field-specific, Fields should be an empty slice.
type ErrorViolation struct {
	// Code is a stable machine-readable violation identifier.
	//
	// For binding validation errors this may be the validator tag, such as
	// "required", "min", or "email". For domain validation errors it should be
	// a stable business-rule code.
	Code string `json:"code"`

	// Message is a human-readable explanation of the violation.
	//
	// Message may change over time and should not be used as a stable client
	// contract.
	Message string `json:"message"`

	// Fields contains the request fields related to this violation.
	//
	// For rule-level violations that are not field-specific, Fields should be an
	// empty slice rather than nil.
	Fields []string `json:"fields"`
}
