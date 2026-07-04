// Package hmac provides a peppered HMAC-SHA256 implementation of the core
// fingerprint Hasher port.
//
// The pepper is a server-side secret held outside the hashed storage, so a
// leaked digest table alone cannot be used to confirm guessed raw values.
// High-entropy secrets hashed with a fast keyed digest are sufficient when
// the pepper stays confidential.
package hmac
