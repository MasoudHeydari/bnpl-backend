// Package checkoutotp generates one-time codes for hosted checkout phone
// verification and builds the domain-separated material they are hashed over.
//
// The plaintext code is sent to the customer by SMS and never persisted. Only
// a keyed hash (HMAC) of DomainSeparatedMaterial is stored, so a leaked
// verifier cannot be replayed against a different checkout session.
package checkoutotp
