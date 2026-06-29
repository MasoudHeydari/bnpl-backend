// Package health defines lifecycle contracts used to expose the application's
// operational health.
//
// The interfaces in this package describe application state only. Adapters are
// responsible for translating that state into transport- or platform-specific
// health endpoints (for example HTTP probes, Kubernetes, systemd, or other
// process supervisors).
package health
