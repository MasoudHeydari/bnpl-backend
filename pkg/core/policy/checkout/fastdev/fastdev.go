//go:build development

// Package fastdev provides a Development-only checkout policy with a five
// calendar-day repayment cadence for exercising reminder, grace, default, and
// recovery flows without waiting Jalali months. Production catalogs must not
// register this policy.
package fastdev
