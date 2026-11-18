// Package businessrule defines the build-time compiled Checkout Business Rule
// catalog.
//
// A Checkout Business Rule is a versioned, launch-wide constraint applied on
// top of a Checkout Policy (for example "finance the full purchase or reject
// it"). The catalog is compiled into the binary; there is no runtime
// configuration. Every persisted checkout decision allocation records the rule
// id and version that governed it so decisions replay and audit reliably.
package businessrule
