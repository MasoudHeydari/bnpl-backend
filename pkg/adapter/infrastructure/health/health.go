// Package health provides infrastructure implementations for the application's
// operational health contracts.
//
// The package contains implementations for lifecycle state management and
// dependency health checks used by the health check use case. It intentionally
// keeps infrastructure concerns, such as databases, caches, and message
// brokers, outside the application layer while exposing them through the
// contracts defined in pkg/core/health.
//
// Typical implementations include:
//
//   - AtomicLifecycleState: Tracks the application's lifecycle during graceful
//     shutdown.
//   - CompositeDependencyChecker: Aggregates multiple dependency checks into a
//     single readiness evaluation.
//   - DatabaseDependencyChecker: Verifies database availability.
//   - MessagingDependencyChecker: Verifies Kafka availability.
//   - RedisDependencyChecker: Verifies Redis availability.
//
// Additional dependency checkers can be added without modifying the health
// check use case by implementing the core DependencyChecker interface.
package health
