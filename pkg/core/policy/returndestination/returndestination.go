// Package returndestination defines pure validation rules for merchant browser
// return destinations.
//
// Destination-name grammar and return-URL structure are domain invariants.
// Selecting a URL scheme policy profile from runtime environment is
// Config/Bootstrap composition, not this package. Webhook destination policy
// lives separately under pkg/core/policy/webhook.
package returndestination
