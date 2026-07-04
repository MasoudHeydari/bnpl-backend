// Package errorreporting defines the application-facing contract for reporting
// unexpected operational failures to an external observability backend.
//
// The contract intentionally lives in core while concrete vendors (for example
// Sentry) stay in infrastructure adapters. Business logic should not depend on
// a concrete error-reporting SDK.
package errorreporting
