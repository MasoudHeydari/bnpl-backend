// Package sha256 provides an unkeyed SHA-256 implementation of the core
// fingerprint Hasher port.
//
// It hashes raw material into stable self-describing digests used for
// fingerprints, idempotency, and duplicate detection. Callers that need a
// keyed digest should use the HMAC adapter instead.
package sha256
