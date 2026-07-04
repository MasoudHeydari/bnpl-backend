// Package checkoutsessionrs exposes the merchant checkout handoff over HTTP.
//
// The package keeps the external partner contract deliberately narrow: partner
// responses carry only checkout identity, state, hosted URL, and expiry. Credit
// and offer economics are returned only on the customer-authenticated start path
// and on the trusted internal create path.
package checkoutsessionrs
