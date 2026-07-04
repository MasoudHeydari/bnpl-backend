// Package checkoutotp defines the immutable security terms of the hosted
// external checkout phone one-time code.
//
// Terms are product security policy, not deployment configuration: operators
// cannot loosen the code length, lifetime, attempt budget, or send limits via
// environment variables. A new policy version is added as a new constructor
// (V2, ...) when the terms change.
//
// The code generator and its HMAC material live in pkg/core/checkoutotp; this
// package only states how strict the checkout must be.
package checkoutotp
