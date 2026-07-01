// Package messaging defines broker-independent messaging contracts.
//
// The core layer depends only on these abstractions and must not import concrete
// broker clients. Adapter packages translate broker messages into Message
// values.
package messaging
