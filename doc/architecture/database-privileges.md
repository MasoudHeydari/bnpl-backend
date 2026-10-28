# Managed Runtime database privileges

The existing `invariants` package now contains `privilege_registry.go` and
`privilege_verify.go`, alongside the unchanged trigger registry/verifier.
The registry describes expected state. Only reviewed versioned SQL migrations
install grants. Startup/CLI verification reads catalogs and effective privileges;
it never intercepts individual SQL commands or grants/revokes privileges.
PostgreSQL itself rejects forbidden operations at execution time.

## Reviewed matrix

S=SELECT, I=INSERT, U=table UPDATE. An explicit column list means UPDATE is
allowed only for those columns. **No table grants DELETE or TRUNCATE**, and none
grants REFERENCES or TRIGGER. Table UPDATE is deliberately limited to decisions.
All other updates are column grants derived from the current SQL write paths.

- `migrations`: SELECT.
- `integration_outbox`: SELECT, INSERT. UPDATE columns: `attempt_count`, `last_error`, `lock_version`, `locked_at`, `locked_by`, `next_attempt_at`, `published_at`, `status`.
- `integration_inbox`: SELECT, INSERT. UPDATE columns: `failed_at`, `failure_code`, `failure_message`, `processed_at`, `status`, `updated_at`.
- `integration_inbox_conflicts`: SELECT, INSERT.
- `saga_instances`: SELECT, INSERT. UPDATE columns: `current_step`, `finished_at`, `last_error`, `lease_until`, `locked_by`, `next_attempt_at`, `phase`, `status`, `step_attempt_count`, `updated_at`, `version`.
- `saga_forward_step_completions`: SELECT, INSERT. UPDATE columns: `compensation_resolved_at`, `compensation_state`.
- `decisions`: SELECT, INSERT, UPDATE.
- `customer_profiles`: SELECT, INSERT. UPDATE columns: `external_customer_id`, `jurisdiction`, `status`, `updated_at`.
- `policy_assignments`: SELECT, INSERT. UPDATE columns: `updated_at`, `valid_until`.
- `policy_assignment_imports`: SELECT, INSERT. UPDATE columns: `completed_at`, `failure_count`, `status`, `success_count`.
- `policy_assignment_import_items`: SELECT, INSERT. UPDATE columns: `error_code`, `error_message`, `status`, `updated_at`.
- `merchant_profiles`: SELECT, INSERT. UPDATE columns: `id`.
- `merchant_branch_profiles`: SELECT, INSERT. UPDATE columns: `offer_preparation_mode`, `updated_at`.
- `merchant_pos_terminals`: SELECT. UPDATE columns: `id`.
- `merchant_online_stores`: SELECT. UPDATE columns: `id`.
- `credit_accounts`: SELECT, INSERT. UPDATE columns: `effective_from`, `expires_at`, `exposure_amount_minor`, `last_limit_decision_id`, `limit_amount_minor`, `revoked_at`, `status`, `updated_at`, `version`.
- `credit_transactions`: SELECT, INSERT.
- `contracts`: SELECT, INSERT. UPDATE columns: `activated_at`, `canceled_at`, `cancellation_context_id`, `cancellation_financial_core_accepted_at`, `cancellation_financial_core_idempotency_key`, `cancellation_idempotency_key`, `cancellation_reason`, `cancellation_request_fingerprint`, `cancellation_requested_at`, `cancellation_tenant_id`, `completed_at`, `financial_invoice_id`, `financial_merchant_entitlement_id`, `financial_payment_attempt_id`, `financial_payment_expires_at`, `financial_payment_intent_id`, `financial_payment_link`, `financial_payment_reused_idempotent_result`, `financial_payment_session_id`, `financial_receivable_id`, `payment_id`, `payment_reference`, `remaining_principal_amount_minor`, `status`, `successful_financial_payment_attempt_id`, `updated_at`.
- `installments`: SELECT, INSERT. UPDATE columns: `contract_id`, `due_at`, `financial_revision`, `paid_amount_minor`, `paid_at`, `status`, `updated_at`.
- `installment_charges`: SELECT, INSERT. UPDATE columns: `charge_status`, `paid_at`, `updated_at`.
- `payment_attempts`: SELECT, INSERT. UPDATE columns: `applied_at`, `cancel_return_url`, `canceled_at`, `context_id`, `failed_at`, `failure_code`, `failure_return_url`, `financial_collection_obligation_id`, `financial_outcome_resolved`, `financial_paid_at`, `financial_payment_attempt_id`, `financial_payment_expires_at`, `financial_payment_intent_id`, `financial_payment_session_id`, `last_reconciliation_error`, `manual_review_required_at`, `next_reconciliation_at`, `payment_url`, `provider_code`, `reconciliation_attempt_count`, `reconciliation_reason`, `reconciliation_requested_at`, `reconciliation_resolved_at`, `state`, `submitted_at`, `success_return_url`, `tenant_id`, `updated_at`.
- `payment_attempt_items`: SELECT, INSERT.
- `checkout_decision_allocations`: SELECT, INSERT. UPDATE columns: `decision_id`.
- `merchant_return_destinations`: SELECT, INSERT. UPDATE columns: `enabled`, `return_url`, `updated_at`.
- `merchant_webhook_destinations`: SELECT, INSERT. UPDATE columns: `enabled`, `signing_secret_ciphertext`, `signing_secret_key_id`, `updated_at`, `webhook_url`.
- `checkout_sessions`: SELECT, INSERT. UPDATE columns: `canceled_at`, `cancellation_reason`, `completed_at`, `confirmation_attempt_id`, `contract_id`, `customer_id`, `decision_id`, `evaluation_generation`, `expires_at`, `status`, `updated_at`.
- `otp_challenges`: SELECT, INSERT. UPDATE columns: `attempt_count`, `context_version`, `expires_at`, `issue_id`, `next_issue_at`, `notification_accepted_at`, `notification_request_id`, `send_count`, `updated_at`, `verified_at`, `verifier`.
- `otp_rate_limits`: SELECT, INSERT. UPDATE columns: `send_count`, `updated_at`, `window_started_at`.
- `merchant_terminals`: SELECT, INSERT. UPDATE columns: `id`.
- `merchant_terminal_credentials`: SELECT, INSERT. UPDATE columns: `retire_at`, `retired_at`, `status`, `updated_at`.
- `merchant_terminal_issuance_deliveries`: SELECT, INSERT. UPDATE columns: `api_key_ciphertext`, `api_key_encryption_key_id`, `consumed_at`, `purged_at`.
- `merchant_terminal_permissions`: SELECT, INSERT. UPDATE columns: `revoked_at`.
- `short_links`: SELECT, INSERT. UPDATE columns: `destination_url`, `revoked_at`, `updated_at`.
- `repayment_reminders`: SELECT, INSERT. UPDATE columns: `id`.
- `default_episodes`: SELECT, INSERT. UPDATE columns: `closing_decision_id`, `recovered_at`, `status`, `updated_at`.
- `default_episode_triggers`: SELECT, INSERT.
- `customer_restrictions`: SELECT, INSERT. UPDATE columns: `released_at`, `releasing_decision_id`, `status`, `updated_at`.
- `collections_cases`: SELECT, INSERT.
- `collections_case_events`: SELECT, INSERT.

The deliberate decisions UPDATE exception supports
`decisionrp.LockLiveCheckoutDecisionByID` without refactoring Checkout locks.
Decisions remain immutable by business convention; this ACL does not enforce
that immutability. Do not add mutation repositories under cover of the exception.
`checkout_decision_allocations` uses UPDATE(decision_id) solely to preserve its
locking select. `merchant_profiles`, `merchant_terminals`, `repayment_reminders`,
`merchant_pos_terminals` and `merchant_online_stores` similarly use UPDATE(id).
For permission replacement, merchant_terminals also writes id=id as a parent
serialization boundary so an old repeatable-read snapshot fails instead of
combining stale grants. No key value changes. These narrow key-column grants
permit locking and would also permit direct key
updates, so do not claim database immutability for them. No repository mutates
these keys. Financial tables retain cross-table trigger enforcement in addition
to their column allowlists. `installments.financial_revision` is writable because
invoker trigger functions must serialize financial child mutations on the parent.

## Installation and future changes

Migration 000004 retains permission assignments with nullable revoked_at and a
partial active uniqueness index. Replacement locks the stable terminal parent,
validates the complete input, revokes removed active assignments, inserts missing
active grants and commits atomically. All authorization reads filter revoked_at
IS NULL. Regrant creates a new assignment, preserving the revoked history.
No DELETE is used; concurrent replacements end with one complete set.

Migration 000005 revokes old table, column, sequence, function and schema grants
from PUBLIC/bnpl, grants schema USAGE plus only the explicit allowlist, and sets
owner-created future objects to default deny. Public function EXECUTE defaults
are revoked at the creating owner role's global default scope, because a
schema-local revoke cannot undo PostgreSQL's global PUBLIC EXECUTE default.
No blanket current/future-table write grant is installed. Attached migration-ID
sequences are owner-only. Runtime does not insert migration metadata.

For a new table or privilege change: inspect all SQL and locking requirements,
update typed registry metadata with a reason and owning migration, add a new
incremental SQL migration with exact GRANT/REVOKE statements, update required
fingerprints and add actual-role tests. Editing registry metadata alone never
changes permissions. A new unregistered table has no Runtime access and makes
verification fail until the policy is reviewed. Previous migrations are immutable.

## Effective read-only verification

Runtime authenticates and executes as bnpl, then verifies trigger definitions
with the existing body fingerprints, clean migration/checksum compatibility,
permission-history schema, all registered tables and required columns, effective
SELECT/INSERT/UPDATE/REFERENCES at table and column scope, DELETE/TRUNCATE/TRIGGER,
schema USAGE/no CREATE, no database/public-schema CREATE, no ownership of managed
relations/functions/types, no privileged role attributes or memberships, no
permission delegation, controlled migrator membership and NOLOGIN owner.
It checks owner default ACLs and function EXECUTE; function overloads do not gain
access merely by sharing a managed name. PUBLIC/inherited rights are included by
PostgreSQL effective-privilege functions, rather than comparing only raw ACLs.
Missing/disabled/changed triggers still fail the original integrity verifier.

DB Admin uses the same catalog checks under migrator/owner and ownership preflight
under admin. Detailed managed-role/membership/ownership/default ACL checks are
available through PostgreSQL public catalogs even to normal Runtime; no extra
Runtime administrative privileges were added. The verifier does not audit
unrelated schemas, operating-system/database proxy settings, or claim to verify
the platform admin's entire cluster-wide authority. Runtime TEMPORARY access is also prohibited, including through PUBLIC. Because
revoking the database-wide PUBLIC default affects other users, it is an explicit
platform prerequisite rather than a silent cross-schema bootstrap side effect.
Managed CREATE, ownership and administrative SET ROLE paths are prohibited.

Investigate drift with `db ownership plan` (safe role/table/column ACL inventory)
and PostgreSQL catalogs/has_table_privilege/has_column_privilege/pg_has_role.
Do not fix it through startup or blanket GRANT ALL. Add a reviewed corrective
migration, or explicitly correct unauthorized role/platform grants with the DBA.
Table-level REVOKE UPDATE can also remove column grants; restore only the reviewed
column allowlist and verify again. All runtime safety failures occur before
listeners and workers start, with existing resource cleanup preserved.
