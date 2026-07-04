// Package policyassignmentuc orchestrates runtime policy membership imports.
//
// Trusted-customer bulk admission treats the operational CSV as the
// authoritative membership source (no Identity Service call at import time),
// then persists assignment history and durable per-row import outcomes. Policy
// version selection remains code-owned; this use case only records which
// reviewed policy refs apply to which subjects.
package policyassignmentuc
