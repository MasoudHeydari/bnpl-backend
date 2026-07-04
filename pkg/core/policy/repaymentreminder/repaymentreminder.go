// Package repaymentreminder defines versioned business rules for repayment
// reminder eligibility, scheduling, and customer-day message limits.
//
// Policy implementations are pure and deterministic: they receive all facts
// explicitly and must not query databases, publish messages, call external
// services, read application configuration, or call time.Now.
package repaymentreminder
