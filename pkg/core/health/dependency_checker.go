package health

import "context"

// DependencyChecker verifies whether the application's required runtime
// dependencies are available.
//
// A successful check indicates that the application is ready to accept new
// work. Implementations may verify one or more dependencies, such as a
// database, cache, message broker, or external service, depending on the
// application's requirements.
//
// The interface intentionally abstracts the underlying infrastructure so the
// health check use case remains independent of concrete technologies.
type DependencyChecker interface {
	// Check verifies that all required runtime dependencies are available.
	//
	// It returns nil when the application is ready to accept new work.
	// Otherwise, it returns an error describing the unavailable dependency.
	Check(ctx context.Context) error
}
