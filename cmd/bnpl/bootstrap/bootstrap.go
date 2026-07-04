// Package bootstrap composes configuration, infrastructure, repositories,
// use cases, background tasks, and HTTP routes for the BNPL application.
//
// It belongs to the adapter layer and orchestrates startup without defining
// domain rules. Runtime commands invoke Boot or business administration workflows.
// Database administration has an independent cmd/bnpl-dbadmin composition root.
package bootstrap
