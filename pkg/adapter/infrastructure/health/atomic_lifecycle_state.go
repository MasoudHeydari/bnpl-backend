// Package health provides infrastructure adapters used by health probes.
package health

import (
	"sync/atomic"

	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
)

// Verify AtomicLifecycleState implements the core health lifecycle contract.
var _ corehealth.LifecycleState = (*AtomicLifecycleState)(nil)

// AtomicLifecycleState tracks the application's lifecycle state during graceful
// shutdown.
//
// It stores only process-local state. Platform-specific health semantics,
// such as readiness or liveness, should be implemented by the adapter that
// exposes health endpoints.
type AtomicLifecycleState struct {
	draining         atomic.Bool
	shutdownTimedOut atomic.Bool
}

// NewLifecycleState returns a lifecycle state tracker for graceful shutdown signaling.
func NewLifecycleState() *AtomicLifecycleState {
	return &AtomicLifecycleState{}
}

// StartDraining marks the application as no longer accepting new work.
func (g *AtomicLifecycleState) StartDraining() {
	g.draining.Store(true)
}

// MarkShutdownTimedOut marks that graceful shutdown exceeded its deadline.
func (g *AtomicLifecycleState) MarkShutdownTimedOut() {
	g.shutdownTimedOut.Store(true)
}

// IsDraining reports whether the application is draining in-flight work.
func (g *AtomicLifecycleState) IsDraining() bool {
	return g.draining.Load()
}

// IsShutdownTimedOut reports whether graceful shutdown exceeded its deadline.
func (g *AtomicLifecycleState) IsShutdownTimedOut() bool {
	return g.shutdownTimedOut.Load()
}
