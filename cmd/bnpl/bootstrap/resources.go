package bootstrap

import (
	"errors"
	"fmt"
)

// releaseResources closes process-owned shared dependencies exactly once.
//
// Ownership: App owns the shared dependency graph (outbound HTTP client,
// messaging, cache, external transports, database pool). Ephemeral runtime
// surfaces started by Run (HTTP/metrics listeners, background workers) are
// stopped by shutdown before this method runs.
//
// Close and the graceful-shutdown path both call this so startup rollback and
// normal termination share one cleanup sequence and cannot double-close.
func (a *App) releaseResources(record func(step string, err error)) error {
	if a == nil {
		return nil
	}

	a.releaseOnce.Do(func() {
		a.releaseErr = a.closeSharedResources(record)
	})

	return a.releaseErr
}

// Close releases all shared resources owned by App.
//
// It is safe to call more than once: the first call performs cleanup and later
// calls return the same joined error. Run's graceful shutdown path also uses
// this ownership so callers do not need a second Close after a clean stop.
func (a *App) Close() error {
	return a.releaseResources(nil)
}

func (a *App) closeSharedResources(record func(step string, err error)) error {
	if record == nil {
		record = func(string, error) {}
	}

	var err error
	step := func(name string, closeErr error) {
		record(name, closeErr)
		if closeErr != nil {
			err = errors.Join(err, fmt.Errorf("%s: %w", name, closeErr))
		}
	}

	// Reverse dependency order: dependents first, database last. HTTP handlers,
	// workers, and external calls may still have been finishing just before
	// shutdown invoked this method; by then listeners and workers are stopped.
	step("http_client", a.Infrastructure.closeHTTPClient())
	step("publisher", a.Infrastructure.closePublisher())
	step("subscriber", a.Infrastructure.closeSubscriber())
	step("messaging_health", a.Infrastructure.closeMessagingHealth())
	step("cache", a.Infrastructure.closeCache())
	step("task", a.Infrastructure.closeTask())
	step("external", a.External.Close())
	step("database", a.closePool())

	// Keep error reporting alive until every other shared dependency has been
	// released so shutdown failures can still be reported before transport close.
	step("error_reporter", a.Infrastructure.closeErrorReporter())

	return err
}

func (a *App) closePool() error {
	if a.Pool == nil {
		return nil
	}

	err := a.Pool.Close()
	a.Pool = nil

	return err
}
