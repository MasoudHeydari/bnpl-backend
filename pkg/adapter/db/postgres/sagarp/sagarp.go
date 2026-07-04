// Package sagarp provides the PostgreSQL adapter for durable Saga process state.
//
// The adapter persists Saga snapshots, worker leases, fencing versions, and
// forward-step completion history. Workflow semantics remain outside this
// package and are defined by the core Saga contracts and their application
// implementations.
package sagarp
