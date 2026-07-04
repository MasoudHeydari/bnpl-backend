// Package merchantterminaluc implements external partner terminal credentials.
//
// A merchant owns one terminal. A partner authenticates a request with the
// terminal's public numeric id and a secret API key. Only a keyed hash of the
// key is stored; the plaintext key is returned exactly once by Provision or
// Rotate and is otherwise replayable only from a short-lived encrypted delivery
// so idempotent retries are safe.
//
// Credentials are versioned. Rotation keeps the previous credential accepted
// for a bounded grace period so partners can deploy the new key without
// downtime.
package merchantterminaluc
