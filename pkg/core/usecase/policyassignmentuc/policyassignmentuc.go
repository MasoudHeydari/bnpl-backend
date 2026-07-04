// Package policyassignmentuc orchestrates runtime policy membership imports.
//
// Trusted-customer bulk admission reads Iranian mobile numbers from CSV,
// resolves each mobile through Customer Reference Service outside any database
// transaction, and persists assignment history plus durable per-row import
// outcomes for ACTIVE customers only. Policy version selection remains
// code-owned via policy.TrustedCustomerBundle(); this use case only records
// which reviewed policy refs apply to which ExternalCustomerID subjects.
package policyassignmentuc
