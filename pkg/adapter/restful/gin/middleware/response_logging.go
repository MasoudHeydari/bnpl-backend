package middleware

import (
	"encoding/json"
	"mime"
	"net/http"
	"strings"
)

// Explicit body-omission / projection reason codes for AccessLogger response
// logging. These values are stable operational identifiers for dashboards and
// must remain distinguishable from each other.
//
// Invariant: every outcome where a body existed but was not projected must set
// body_present=true (or equivalent) with a non-empty reason. Logging
// http.response_body as null/absent is intentionally avoided because operators
// cannot tell "no body", "withheld", and "feature off" apart.
const (
	// responseBodyReasonLogged means an allowlisted scalar projection was
	// attached under fields. filtered=true always accompanies this outcome so
	// readers never mistake the projection for the full response body.
	responseBodyReasonLogged = "logged"

	// responseBodyReasonEmptyBody means no response body bytes were forwarded
	// (and capture was not truncated). Typical for 204 / header-only replies.
	responseBodyReasonEmptyBody = "empty_body"

	// responseBodyReasonNoAllowlistPolicy means JSON was captured but no static
	// policy exists for method+FullPath, or the matched policy has no field
	// paths for the status class. Fail-closed: never log the raw body.
	responseBodyReasonNoAllowlistPolicy = "no_allowlist_policy"

	// responseBodyReasonUnsupportedContentType means the body is not ordinary
	// JSON (application/json or +json). Non-JSON bodies are never projected.
	responseBodyReasonUnsupportedContentType = "unsupported_content_type"

	// responseBodyReasonBodyTooLarge means forwarded bytes exceeded the capture
	// limit. A partial buffer must not be parsed as a complete JSON document.
	responseBodyReasonBodyTooLarge = "body_too_large"

	// responseBodyReasonInvalidJSON means the retained bytes are not valid JSON.
	responseBodyReasonInvalidJSON = "invalid_json"

	// responseBodyReasonUnexpectedResponseShape means the JSON root is not an
	// object, a required allowlisted path is missing, or a path value is not a
	// safe scalar (object/array/etc.). Fail-closed.
	responseBodyReasonUnexpectedResponseShape = "unexpected_response_shape"

	// responseBodyReasonStreaming means capture was abandoned after Hijack (or
	// an equivalent skip). The HTTP body is no longer a normal response stream.
	responseBodyReasonStreaming = "streaming_response"

	// responseBodyReasonEncodedResponseUnsupported means Content-Encoding is
	// set to something other than identity. This code refuses to decompress
	// attacker- or proxy-controlled encodings merely for logging.
	responseBodyReasonEncodedResponseUnsupported = "encoded_response_unsupported"

	// responseBodyReasonCaptureDisabled is a defensive internal outcome when
	// projection is requested but no capture wrapper was installed.
	responseBodyReasonCaptureDisabled = "capture_disabled"
)

// ResponseLoggingOptions configures optional safe response-body enrichment of
// AccessLogger records.
//
// The feature is disabled by default. When enabled, only explicitly allowlisted
// projections for known routes may appear in logs. Unknown routes and unknown
// fields fail closed with an explicit omission reason — never a raw body dump.
//
// Security principle: a response that is not explicitly approved for logging
// must never fall back to logging the raw body. Blacklist-only redaction is
// intentionally rejected because newly introduced sensitive fields would leak
// until someone remembers to add them to a deny list.
type ResponseLoggingOptions struct {
	// Enabled controls whether AccessLogger captures and projects response bodies.
	// When false, AccessLogger must not install a response capture wrapper so
	// the hot path retains its prior allocation characteristics.
	Enabled bool

	// MaxBodyBytes caps how many response bytes are retained for projection.
	// Non-positive values fall back to defaultResponseBodyCaptureLimit.
	// This bound affects log memory only; the client still receives the full body.
	MaxBodyBytes int64
}

// responseBodyLog is the structured AccessLogger attribute under
// http.response_body. Field meanings:
//
//   - BodyPresent: true if the HTTP exchange produced (or attempted to produce)
//     a response body. Distinct from Logged — a sensitive body can be present
//     while nothing from it is recorded.
//   - Logged: true only when Fields contains an allowlisted projection.
//   - Filtered: true when Logged and the projection is a subset of the body.
//     Callers must not treat Fields as the full response.
//   - Reason: stable machine-readable outcome (see responseBodyReason* consts).
//   - Truncated: capture limit was exceeded (prefix only; never parse as complete).
//   - ContentType / CapturedSize / LimitBytes: diagnostic context for omission.
//   - Fields: allowlisted scalars only; never the raw JSON document.
type responseBodyLog struct {
	BodyPresent  bool           `json:"body_present"`
	Logged       bool           `json:"logged"`
	Filtered     bool           `json:"filtered,omitempty"`
	Reason       string         `json:"reason"`
	Truncated    bool           `json:"truncated,omitempty"`
	ContentType  string         `json:"content_type,omitempty"`
	CapturedSize int            `json:"captured_size,omitempty"`
	LimitBytes   int64          `json:"limit_bytes,omitempty"`
	Fields       map[string]any `json:"fields,omitempty"`
}

// asAttrs flattens the projection into a map suitable for structured logger
// key/value attributes. Omitted empty optional fields keep the access record
// small when the body was empty or logging was skipped early.
func (r responseBodyLog) asAttrs() map[string]any {
	out := map[string]any{
		"body_present": r.BodyPresent,
		"logged":       r.Logged,
		"reason":       r.Reason,
	}

	if r.Filtered {
		out["filtered"] = true
	}

	if r.Truncated {
		out["truncated"] = true
	}

	if r.ContentType != "" {
		out["content_type"] = r.ContentType
	}

	if r.CapturedSize > 0 {
		out["captured_size"] = r.CapturedSize
	}

	if r.LimitBytes > 0 {
		out["limit_bytes"] = r.LimitBytes
	}

	if len(r.Fields) > 0 {
		out["fields"] = r.Fields
	}

	return out
}

// projectResponseBody applies the fail-closed Production response logging
// decision process. Check order is load-bearing:
//
//  1. skipReason — transport-level capture abandonment (e.g. hijack) wins first.
//  2. empty body — distinguish truly empty responses from withheld bodies.
//  3. truncated — never parse a size-capped prefix as complete JSON.
//  4. Content-Encoding — refuse compressed/encoded bodies without decompressing.
//  5. Content-Type — only ordinary JSON is eligible for projection.
//  6. allowlist policy — unknown method+route templates fail closed.
//  7. JSON validity / object root — malformed or non-object JSON fails closed.
//  8. status-class field paths — success vs error policies differ by design.
//  9. path extraction + scalar type checks — missing paths or nested values fail closed.
//
// Unknown JSON keys present alongside approved fields are ignored (not copied).
// Nested objects and arrays are never accepted as leaf values for an approved
// path, so a policy entry cannot accidentally pull an entire subtree.
func projectResponseBody(
	method string,
	route string,
	status int,
	header http.Header,
	body []byte,
	truncated bool,
	skipReason string,
	limit int64,
) responseBodyLog {
	contentType := header.Get("Content-Type")

	if skipReason != "" {
		return responseBodyLog{
			BodyPresent:  len(body) > 0 || truncated,
			Logged:       false,
			Reason:       skipReason,
			ContentType:  contentType,
			CapturedSize: len(body),
			LimitBytes:   limit,
			Truncated:    truncated,
		}
	}

	if len(body) == 0 && !truncated {
		return responseBodyLog{
			BodyPresent: false,
			Logged:      false,
			Reason:      responseBodyReasonEmptyBody,
		}
	}

	if truncated {
		return responseBodyLog{
			BodyPresent:  true,
			Logged:       false,
			Reason:       responseBodyReasonBodyTooLarge,
			Truncated:    true,
			ContentType:  contentType,
			CapturedSize: len(body),
			LimitBytes:   limit,
		}
	}

	if encoding := strings.TrimSpace(header.Get("Content-Encoding")); encoding != "" &&
		!strings.EqualFold(encoding, "identity") {
		return responseBodyLog{
			BodyPresent:  true,
			Logged:       false,
			Reason:       responseBodyReasonEncodedResponseUnsupported,
			ContentType:  contentType,
			CapturedSize: len(body),
			LimitBytes:   limit,
		}
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(contentType)
	}

	if mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
		return responseBodyLog{
			BodyPresent:  true,
			Logged:       false,
			Reason:       responseBodyReasonUnsupportedContentType,
			ContentType:  contentType,
			CapturedSize: len(body),
			LimitBytes:   limit,
		}
	}

	policy, ok := lookupResponseLogPolicy(method, route)
	if !ok {
		return responseBodyLog{
			BodyPresent:  true,
			Logged:       false,
			Reason:       responseBodyReasonNoAllowlistPolicy,
			ContentType:  contentType,
			CapturedSize: len(body),
			LimitBytes:   limit,
		}
	}

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return responseBodyLog{
			BodyPresent:  true,
			Logged:       false,
			Reason:       responseBodyReasonInvalidJSON,
			ContentType:  contentType,
			CapturedSize: len(body),
			LimitBytes:   limit,
		}
	}

	root, ok := parsed.(map[string]any)
	if !ok {
		return responseBodyLog{
			BodyPresent:  true,
			Logged:       false,
			Reason:       responseBodyReasonUnexpectedResponseShape,
			ContentType:  contentType,
			CapturedSize: len(body),
			LimitBytes:   limit,
		}
	}

	// Status class selects which allowlisted paths apply. A 4xx/5xx body uses
	// ErrorFields even if it somehow resembles a success envelope, preventing
	// accidental logging of error messages / details that are never approved.
	paths := policy.SuccessFields
	if status >= http.StatusBadRequest {
		paths = policy.ErrorFields
	}

	if len(paths) == 0 {
		return responseBodyLog{
			BodyPresent:  true,
			Logged:       false,
			Reason:       responseBodyReasonNoAllowlistPolicy,
			ContentType:  contentType,
			CapturedSize: len(body),
			LimitBytes:   limit,
		}
	}

	fields, ok := extractAllowlistedFields(root, paths)
	if !ok {
		return responseBodyLog{
			BodyPresent:  true,
			Logged:       false,
			Reason:       responseBodyReasonUnexpectedResponseShape,
			ContentType:  contentType,
			CapturedSize: len(body),
			LimitBytes:   limit,
		}
	}

	return responseBodyLog{
		BodyPresent:  true,
		Logged:       true,
		Filtered:     true,
		Reason:       responseBodyReasonLogged,
		ContentType:  contentType,
		CapturedSize: len(body),
		LimitBytes:   limit,
		Fields:       fields,
	}
}

// extractAllowlistedFields builds a new map containing only the approved
// scalar paths. Every listed path must resolve; partial projections are
// rejected so operators never see an incomplete "success" view of a broken
// or unexpected envelope.
func extractAllowlistedFields(root map[string]any, paths []fieldPath) (map[string]any, bool) {
	out := make(map[string]any)

	for _, path := range paths {
		if len(path) == 0 {
			return nil, false
		}

		value, ok := lookupPath(root, path)
		if !ok {
			return nil, false
		}

		if !isSafeScalar(value) {
			return nil, false
		}

		setPath(out, path, value)
	}

	return out, true
}

func lookupPath(root map[string]any, path fieldPath) (any, bool) {
	var current any = root

	for _, key := range path {
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}

		next, exists := asMap[key]
		if !exists {
			return nil, false
		}

		current = next
	}

	return current, true
}

func setPath(root map[string]any, path fieldPath, value any) {
	current := root

	for i, key := range path {
		if i == len(path)-1 {
			current[key] = value
			return
		}

		next, ok := current[key].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[key] = next
		}

		current = next
	}
}

// isSafeScalar accepts only JSON leaf types produced by encoding/json
// Unmarshal into any (nil, bool, float64, string). Maps and arrays are
// rejected so an allowlisted path cannot pull nested structures that may
// contain unreviewed sensitive fields.
func isSafeScalar(value any) bool {
	switch value.(type) {
	case nil, bool, float64, string:
		return true

	default:
		// json.Unmarshal uses float64 for numbers. Reject maps/arrays/other
		// types so nested objects cannot bypass the allowlist.
		return false
	}
}
