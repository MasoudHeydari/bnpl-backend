// Package merchantterminal defines the core abstractions for authenticating
// external partner systems with a Terminal ID and API key.
//
// A partner presents a numeric terminal identifier and a secret API key. The
// key is stored only as a keyed digest from fingerprint.Hasher, compared in
// constant time, and resolved to a MerchantPrincipal that downstream code
// reads from the request context. This package stays free of transport and
// storage concerns.
package merchantterminal
