// Package healthcheckuc implements the application health check use case.
//
// The use case evaluates whether the application is ready to accept new work
// and whether the process is still alive. It depends only on small ports for
// lifecycle state and optional infrastructure health checks, remaining
// independent of HTTP, orchestrators, and transport-specific concerns.
package healthcheckuc
