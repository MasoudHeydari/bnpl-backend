// Package webhook defines deterministic destination-security rules for outbound
// merchant webhook delivery.
//
// Rules here are pure: URL structure, host screening, and address classification.
// DNS resolution, dialing, HTTP transport, and DNS-rebinding checks at connect
// time belong to the merchantwebhook adapter. Selecting a policy profile from
// runtime environment is Config/Bootstrap composition, not this package.
package webhook
