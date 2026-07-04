// Package noop provides a deterministic local checkout payment client.
//
// It satisfies the core checkout payment contract without calling Financial
// Core. Use it in local development and tests where the real payment service
// is unavailable.
package noop
