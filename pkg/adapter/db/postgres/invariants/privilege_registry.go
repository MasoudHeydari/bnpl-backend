package invariants

// Operation is a reviewed table privilege; DELETE/TRUNCATE are never supported.
type Operation string

const (
	SelectOperation Operation = "SELECT"
	InsertOperation Operation = "INSERT"
	UpdateOperation Operation = "UPDATE"
)

// TablePrivilegePolicy describes expected effective Runtime permissions, not an
// installer. Initial grants live in 000_schema.sql; post-release policy changes
// require a new migration. See database-privileges.md.
type TablePrivilegePolicy struct {
	Schema, Table, Migration, Description string
	Operations                            []Operation
	UpdateColumns                         []string
}

// PrivilegeRegistry returns a deep copy of the reviewed allowlist.
func PrivilegeRegistry() []TablePrivilegePolicy {
	out := make([]TablePrivilegePolicy, len(privileges))
	for i, p := range privileges {
		out[i] = p
		out[i].Operations = append([]Operation(nil), p.Operations...)
		out[i].UpdateColumns = append([]string(nil), p.UpdateColumns...)
	}
	return out
}

const PrivilegeMigration = "000_schema.sql"

var privileges = []TablePrivilegePolicy{
	{Schema: "bnpl", Table: "migrations", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation}, UpdateColumns: []string{}},
	{Schema: "bnpl", Table: "integration_outbox", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"attempt_count", "last_error", "lock_version", "locked_at", "locked_by", "next_attempt_at", "published_at", "status"}},
	{Schema: "bnpl", Table: "integration_inbox", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"failed_at", "failure_code", "failure_message", "processed_at", "status", "updated_at"}},
	{Schema: "bnpl", Table: "integration_inbox_conflicts", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{}},
	{Schema: "bnpl", Table: "saga_instances", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"current_step", "finished_at", "last_error", "lease_until", "locked_by", "next_attempt_at", "phase", "status", "step_attempt_count", "updated_at", "version"}},
	{Schema: "bnpl", Table: "saga_forward_step_completions", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"compensation_resolved_at", "compensation_state"}},
	{Schema: "bnpl", Table: "decisions", Migration: PrivilegeMigration, Description: "Temporary table UPDATE exception for LockLiveCheckoutDecisionByID; Decisions remain immutable by business convention, not by this ACL.", Operations: []Operation{SelectOperation, InsertOperation, UpdateOperation}, UpdateColumns: []string{}},
	{Schema: "bnpl", Table: "customer_profiles", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"external_customer_id", "jurisdiction", "status", "updated_at"}},
	{Schema: "bnpl", Table: "policy_assignments", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"updated_at", "valid_until"}},
	{Schema: "bnpl", Table: "policy_assignment_imports", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"completed_at", "failure_count", "status", "success_count"}},
	{Schema: "bnpl", Table: "policy_assignment_import_items", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"error_code", "error_message", "status", "updated_at"}},
	{Schema: "bnpl", Table: "merchant_profiles", Migration: PrivilegeMigration, Description: "Minimal UPDATE(id) compatibility exception for existing SELECT FOR UPDATE; no repository key mutation.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"id"}},
	{Schema: "bnpl", Table: "merchant_branch_profiles", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"offer_preparation_mode", "updated_at"}},
	{Schema: "bnpl", Table: "merchant_pos_terminals", Migration: PrivilegeMigration, Description: "Minimal UPDATE(id) compatibility exception for existing SELECT FOR UPDATE; no repository key mutation.", Operations: []Operation{SelectOperation}, UpdateColumns: []string{"id"}},
	{Schema: "bnpl", Table: "merchant_online_stores", Migration: PrivilegeMigration, Description: "Minimal UPDATE(id) compatibility exception for existing SELECT FOR UPDATE; no repository key mutation.", Operations: []Operation{SelectOperation}, UpdateColumns: []string{"id"}},
	{Schema: "bnpl", Table: "credit_accounts", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"effective_from", "expires_at", "exposure_amount_minor", "last_limit_decision_id", "limit_amount_minor", "revoked_at", "status", "updated_at", "version"}},
	{Schema: "bnpl", Table: "credit_transactions", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{}},
	{Schema: "bnpl", Table: "contracts", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"activated_at", "canceled_at", "cancellation_context_id", "cancellation_financial_core_accepted_at", "cancellation_financial_core_idempotency_key", "cancellation_idempotency_key", "cancellation_reason", "cancellation_request_fingerprint", "cancellation_requested_at", "cancellation_tenant_id", "completed_at", "financial_invoice_id", "financial_merchant_entitlement_id", "financial_payment_attempt_id", "financial_payment_expires_at", "financial_payment_intent_id", "financial_payment_link", "financial_payment_reused_idempotent_result", "financial_payment_session_id", "financial_receivable_id", "payment_id", "payment_reference", "remaining_principal_amount_minor", "status", "successful_financial_payment_attempt_id", "updated_at"}},
	{Schema: "bnpl", Table: "installments", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"contract_id", "due_at", "financial_revision", "paid_amount_minor", "paid_at", "status", "updated_at"}},
	{Schema: "bnpl", Table: "installment_charges", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"charge_status", "paid_at", "updated_at"}},
	{Schema: "bnpl", Table: "payment_attempts", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"applied_at", "cancel_return_url", "canceled_at", "context_id", "failed_at", "failure_code", "failure_return_url", "financial_collection_obligation_id", "financial_outcome_resolved", "financial_paid_at", "financial_payment_attempt_id", "financial_payment_expires_at", "financial_payment_intent_id", "financial_payment_session_id", "last_reconciliation_error", "manual_review_required_at", "next_reconciliation_at", "payment_url", "provider_code", "reconciliation_attempt_count", "reconciliation_reason", "reconciliation_requested_at", "reconciliation_resolved_at", "state", "submitted_at", "success_return_url", "tenant_id", "updated_at"}},
	{Schema: "bnpl", Table: "payment_attempt_items", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{}},
	{Schema: "bnpl", Table: "checkout_decision_allocations", Migration: PrivilegeMigration, Description: "Minimal UPDATE(id) compatibility exception for existing SELECT FOR UPDATE; no repository key mutation.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"decision_id"}},
	{Schema: "bnpl", Table: "merchant_return_destinations", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"enabled", "return_url", "updated_at"}},
	{Schema: "bnpl", Table: "merchant_webhook_destinations", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"enabled", "signing_secret_ciphertext", "signing_secret_key_id", "updated_at", "webhook_url"}},
	{Schema: "bnpl", Table: "checkout_sessions", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"canceled_at", "cancellation_reason", "completed_at", "confirmation_attempt_id", "contract_id", "customer_id", "decision_id", "evaluation_generation", "expires_at", "status", "updated_at"}},
	{Schema: "bnpl", Table: "otp_challenges", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"attempt_count", "context_version", "expires_at", "issue_id", "next_issue_at", "notification_accepted_at", "notification_request_id", "send_count", "updated_at", "verified_at", "verifier"}},
	{Schema: "bnpl", Table: "otp_rate_limits", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"send_count", "updated_at", "window_started_at"}},
	{Schema: "bnpl", Table: "merchant_terminals", Migration: PrivilegeMigration, Description: "UPDATE(id) permits locking and a key-preserving id=id write to serialize permission replacements, including repeatable-read snapshots.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"id"}},
	{Schema: "bnpl", Table: "merchant_terminal_credentials", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"retire_at", "retired_at", "status", "updated_at"}},
	{Schema: "bnpl", Table: "merchant_terminal_issuance_deliveries", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"api_key_ciphertext", "api_key_encryption_key_id", "consumed_at", "purged_at"}},
	{Schema: "bnpl", Table: "merchant_terminal_permissions", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"revoked_at"}},
	{Schema: "bnpl", Table: "short_links", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"destination_url", "revoked_at", "updated_at"}},
	{Schema: "bnpl", Table: "repayment_reminders", Migration: PrivilegeMigration, Description: "Minimal UPDATE(id) compatibility exception for existing SELECT FOR UPDATE; no repository key mutation.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"id"}},
	{Schema: "bnpl", Table: "default_episodes", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"closing_decision_id", "recovered_at", "status", "updated_at"}},
	{Schema: "bnpl", Table: "default_episode_triggers", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{}},
	{Schema: "bnpl", Table: "customer_restrictions", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{"released_at", "releasing_decision_id", "status", "updated_at"}},
	{Schema: "bnpl", Table: "collections_cases", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{}},
	{Schema: "bnpl", Table: "collections_case_events", Migration: PrivilegeMigration, Description: "Reviewed business SQL; immutable financial identity is not writable.", Operations: []Operation{SelectOperation, InsertOperation}, UpdateColumns: []string{}},
}

const PrivilegeMigrationSHA256 = "sha256:af4ed884c13eaf64b5c0a7e8ebf3164b68ceec623753ba8763bf54cf57123293"

const PermissionHistoryMigration = "000_schema.sql"
const PermissionHistoryMigrationSHA256 = "sha256:b9cc06a580a24f2173980bd1655692cfe02160e40f3863d96b39a9820fd421df"
