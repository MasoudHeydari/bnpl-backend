// Package otpcode generates one-time codes and builds the domain-separated
// material they are hashed over.
//
// The plaintext code is sent to the customer by SMS and never persisted. Only
// a keyed hash (HMAC) of DomainSeparatedMaterial is stored, so a leaked
// verifier cannot be replayed against a different subject binding. The material
// domain string remains bnpl:otp:v1.
package otpcode
