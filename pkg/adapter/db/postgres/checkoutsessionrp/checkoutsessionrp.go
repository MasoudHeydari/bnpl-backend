// Package checkoutsessionrp implements the PostgreSQL checkout session repository.
//
// It persists merchant checkout handoffs plus the merchant configuration
// tables the handoff boundary depends on: pre-registered return destinations and
// the merchant webhook destination.
package checkoutsessionrp
