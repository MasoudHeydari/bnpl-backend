package health

// LifecycleState represents the application's lifecycle during graceful
// shutdown.
//
// Once draining starts, the application should stop accepting new work while
// allowing in-flight operations to complete. If shutdown cannot finish within
// the configured deadline, the lifecycle transitions to a shutdown-timeout
// state so external systems may decide how to handle the process.
type LifecycleState interface {
	// StartDraining marks the application as no longer accepting new work.
	StartDraining()

	// MarkShutdownTimedOut marks that graceful shutdown exceeded its deadline.
	MarkShutdownTimedOut()

	// IsDraining reports whether the application is draining in-flight work.
	IsDraining() bool

	// IsShutdownTimedOut reports whether graceful shutdown exceeded its deadline.
	IsShutdownTimedOut() bool
}
