// Package circuitbreaker defines the core abstraction for protecting calls to
// unstable or remote dependencies.
//
// The package belongs to the core layer and must not depend on any concrete
// circuit breaker implementation. Implementations live in adapter packages.
package circuitbreaker
