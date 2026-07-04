package serdser

// ErrorDetails is the client-safe structured details contract returned by REST.
type ErrorDetails struct {
	Fields            []string         `json:"fields"`
	Violations        []ErrorViolation `json:"violations"`
	RetryAfterSeconds *int             `json:"retry_after_seconds,omitempty" example:"30"`
}

// ErrorViolation is the localized field-level or rule-level REST failure.
type ErrorViolation struct {
	Code    string   `json:"code" example:"INVALID_ARGUMENT" i18n:"canonical"`
	Message string   `json:"message" example:"currency is invalid" i18n:"localized"`
	Fields  []string `json:"fields"`
}

// ErrorResponse is the platform-wide REST error response contract.
//
// Every non-2xx REST response must use this structure. Message and violation
// messages are localized at the REST boundary from language-neutral cerr keys;
// internal causes are never serialized. TraceID correlates the response with
// server-side logs/traces.
type ErrorResponse struct {
	Code    string        `json:"code" example:"VALIDATION_FAILED" i18n:"canonical"`
	Message string        `json:"message" example:"Request validation failed." i18n:"localized"`
	TraceID string        `json:"trace_id" example:"a1b2c3d4-e5f6-7890-abcd-ef1234567890" format:"uuid"`
	Details *ErrorDetails `json:"details,omitempty"`
}
