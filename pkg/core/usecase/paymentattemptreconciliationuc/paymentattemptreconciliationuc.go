// Package paymentattemptreconciliationuc finds stale payment attempts and
// requests reconciliation with Financial Core.
//
// Background jobs drive this package; it coordinates repositories and outbox
// events without depending on HTTP handlers.
package paymentattemptreconciliationuc
