// Package sagaexecutoruc resumes durable Saga instances from background jobs.
//
// It loads saga definitions from the registry, advances steps transactionally
// where required, and records outcomes without owning domain-specific policies.
package sagaexecutoruc
