// Package outbox provides task jobs for the transactional outbox publisher.
//
// Jobs in this package delegate to the outbox publisher use case to claim and
// publish due integration messages.
package outbox
