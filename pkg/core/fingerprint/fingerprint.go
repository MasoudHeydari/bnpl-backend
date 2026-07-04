// Package fingerprint defines the core port for hashing raw material into a
// self-describing Value and verifying stored digests.
//
// Hasher is the single hashing port: adapters provide unkeyed SHA-256 or
// peppered HMAC-SHA256. Canonical field selection and serialization remain in
// Core use cases; callers typically Validate the returned Value before
// persistence when the digest is used as a business fingerprint.
package fingerprint
