// Package checkoutsessionrs exposes partner and hosted customer checkout over HTTP.
//
// Partner responses stay deliberately narrow: checkout identity, state, hosted
// URL, and expiry. Credit and offer economics are returned only on the
// unauthenticated customer-token surface (GET/START/CONFIRM) after offer
// preparation, never on merchant status or create responses.
package checkoutsessionrs
