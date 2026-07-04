// Package contractcancellation defines the business-facing Financial Core port
// for requesting contract cancellation.
//
// Financial Core is the sole authority for monetary cancellation outcomes.
// BNPL records local canceling intent and correlates requests with a stable
// idempotency identity; it never invents ledger, refund, or reversal truth.
//
// This package must stay transport-neutral: no protobuf, gRPC, or Financial
// Core DTO types belong here.
package contractcancellation
