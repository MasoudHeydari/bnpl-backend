// Package checkoutverification defines the compile-time merchant verification
// policy family for hosted checkout after Financial Core success.
//
// Policies are pure and deterministic: they compute verification deadlines and
// accept/reject verify timing from caller-supplied timestamps. They never
// access a database, clock, network, or merchant-specific override.
package checkoutverification
