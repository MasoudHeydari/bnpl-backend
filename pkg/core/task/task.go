// Package task defines the application-level contract for task execution without
// depending on any specific scheduling, queueing, or orchestration technology.
// It intentionally separates scheduled execution from asynchronous dispatch so
// applications can evolve from in-process execution to distributed systems
// without changing business code.
package task
