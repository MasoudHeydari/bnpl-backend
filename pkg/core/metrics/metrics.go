// Package metrics defines technology-agnostic observability contracts used by
// the application layer.
//
// Adapters in pkg/adapter/infrastructure/metrics implement these contracts.
// Use cases must depend only on these interfaces — never on Prometheus types
// or HTTP exposition details.
package metrics
