// Package metrics provides infrastructure adapters that collect and expose
// application business metrics.
//
// Core use cases depend only on pkg/core/metrics abstractions. Concrete
// providers (noop, prometheus) and the dedicated scrape HTTP server live here
// so Prometheus and HTTP concerns stay out of the domain layer.
package metrics
