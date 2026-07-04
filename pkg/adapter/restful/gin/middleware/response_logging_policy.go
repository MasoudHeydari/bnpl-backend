package middleware

import "strings"

// fieldPath is a JSON object path relative to the response root.
// Example: {"meta", "status"} selects meta.status.
//
// Paths are positive allowlist entries, not JSONPath expressions. Only object
// key traversal is supported. Arrays, wildcards, and recursive descent are
// intentionally unsupported so a single policy line cannot pull unbounded
// nested content.
type fieldPath []string

// responseLogPolicy is a static allowlist for one HTTP method + Gin route
// template (c.FullPath()). Only listed scalar fields may appear in AccessLogger
// response projections. Unknown routes and unknown fields fail closed.
//
// Why method + FullPath (not raw URL paths):
//   - FullPath is the route template Gin matched (e.g. /contracts/:contract_id).
//     Raw paths embed resource IDs, checkout tokens, and other high-cardinality
//     secrets that must never be used as policy keys or log dimensions.
//   - Method separates GET vs POST contracts that share a path prefix but return
//     different envelopes.
//
// Why an explicit allowlist instead of blacklist redaction:
//   - New API fields must not become loggable merely by appearing in a response.
//   - Deny-lists inevitably lag behind new sensitive fields (phones, URLs,
//     tokens, nested payment objects).
//   - This table is a security boundary: changing it requires the same scrutiny
//     as changing authentication middleware, not a casual formatting tweak.
//
// SuccessFields vs ErrorFields:
//   - Success paths typically allow stable machine codes (meta.status / meta.code).
//   - Error paths typically allow only the top-level public error code — never
//     localized messages, violation details, or stack-like diagnostics.
type responseLogPolicy struct {
	Method        string
	Route         string
	SuccessFields []fieldPath
	ErrorFields   []fieldPath
}

// responseLogPolicies is the initial allowlisted Production response projections.
//
// Keep this list small and explicit. Adding a field or route requires a code
// review — new API fields must never become loggable merely by appearing in a
// response body.
//
// Allowed fields today:
//   - Health probes: status only (machine-facing UP/DOWN; no arbitrary error text)
//   - Customer credit GET: meta.status + meta.code on success; code on errors
//   - Customer credit initiate: meta.status + meta.code on success; code on errors
//
// How to add an approved field or route safely:
//  1. Confirm the Gin FullPath template (from route registration), not a sample URL.
//  2. Inspect the real response DTO under pkg/adapter/restful/gin/*rs and serdser.
//  3. Prefer stable machine-readable codes over human messages, URLs, or PII.
//  4. Verify the leaf value is a scalar that cannot embed nested secrets.
//  5. Before merging, verify the field does not reveal personal information,
//     secrets, credentials, payment URLs, checkout tokens, redirect targets,
//     phone numbers, OTPs, or other sensitive context.
//  6. Add focused tests that the new field appears and that neighboring
//     unapproved fields (including newly invented ones) do not.
//  7. Treat the PR as a security review, not a logging convenience change.
//
// Routes absent from this table always project as no_allowlist_policy when
// response logging is enabled — that is the desired default.
var responseLogPolicies = []responseLogPolicy{
	{
		Method:        "GET",
		Route:         "/healthz/live",
		SuccessFields: []fieldPath{{"status"}},
		ErrorFields:   []fieldPath{{"status"}},
	},
	{
		Method:        "GET",
		Route:         "/healthz/ready",
		SuccessFields: []fieldPath{{"status"}},
		ErrorFields:   []fieldPath{{"status"}},
	},
	{
		Method:        "GET",
		Route:         "/api/v1/customer-credit",
		SuccessFields: []fieldPath{{"meta", "status"}, {"meta", "code"}},
		ErrorFields:   []fieldPath{{"code"}},
	},
	{
		Method:        "POST",
		Route:         "/api/v1/customer-credit/initiate",
		SuccessFields: []fieldPath{{"meta", "status"}, {"meta", "code"}},
		ErrorFields:   []fieldPath{{"code"}},
	},
}

// lookupResponseLogPolicy returns the static policy for method+route, if any.
// Matching is exact after normalizing method case; there is no prefix or glob
// matching, so a new route cannot inherit a sibling route's permissions.
func lookupResponseLogPolicy(method, route string) (responseLogPolicy, bool) {
	method = strings.ToUpper(strings.TrimSpace(method))
	route = strings.TrimSpace(route)

	for _, policy := range responseLogPolicies {
		if policy.Method == method && policy.Route == route {
			return policy, true
		}
	}

	return responseLogPolicy{}, false
}

// RegisterResponseLogPolicyForTest installs a temporary allowlist policy used by
// unit tests. It must not be called from production wiring.
//
// The returned restore function reverts the global table. Tests that mutate
// policy state must defer restore() so parallel packages are not poisoned.
func RegisterResponseLogPolicyForTest(policy responseLogPolicy) (restore func()) {
	previous := responseLogPolicies
	responseLogPolicies = append(append([]responseLogPolicy{}, previous...), policy)

	return func() {
		responseLogPolicies = previous
	}
}
