// Package saga defines generic application-level abstractions for implementing
// durable Saga-based workflows.
//
// The package models Saga execution independently of any particular workflow
// engine, scheduler, message broker, transport, or persistence technology.
//
// A Saga is represented as a durable sequence or graph of steps. Each step
// explicitly reports how execution should proceed, including successful
// continuation, waiting, retry, compensation, reconciliation, manual
// intervention, or terminal failure.
//
// Definitions describe the valid structure of a Saga, including its initial
// step, allowed transitions, and compensation strategy. Compensation may be a
// direct operation associated with a completed step or an explicit multi-step
// workflow of its own.
//
// This package defines orchestration contracts only. It does not own business
// rules, persistence, scheduling, transport concerns, or external side effects.
// Those responsibilities are implemented by higher-level application code and
// infrastructure through the appropriate abstractions.
//
// Implementations must assume that steps can be invoked more than once because
// of crashes, retries, lease expiration, lost responses, or other distributed
// system failure modes. Durable state transitions and external side effects
// should therefore be designed with idempotency and concurrency safety in mind.
package saga
