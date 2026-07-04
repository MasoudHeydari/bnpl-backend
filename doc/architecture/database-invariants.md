> Administration commands now belong to `bnpl-dbadmin` with `configs/dbadmin.yaml`.
> Runtime additionally verifies the managed privilege/ownership contract. See
> [roles](database-roles.md), [privileges](database-privileges.md) and
> [deployment](database-deployment.md). Financial integrity definitions now live
> in the initial schema; no separate pre-release migration is installed.

# Managed installment integrity

`paid` is complete, terminal installment settlement. Principal-only settlement
followed by a residual/charge-only payment is unsupported. BNPL computes one
customer cash aggregate and keeps its immutable components; Financial Core (FC)
remains product-agnostic and authoritative for the financial outcome.

## Ownership and installation

Business calculation/validation is owned by Core models and payment/activation
Use Cases. PostgreSQL enforces persisted cross-table integrity even for direct
SQL. The adapter registry in `pkg/adapter/db/postgres/invariants` describes and
verifies those rules; it is not another business policy or migration engine.

The initial `pkg/adapter/db/postgres/migrationsrp/000_schema.sql` installs these
objects on a freshly initialized database. There is no separate pre-release
migration. Initial schema setup does not repair or delete business rows.

The three managed invariants are:

- `installment_no_charges_after_paid`: no charge insertion, mutation (including
  metadata), movement or deletion after settlement. Charge ownership is always
  immutable and historical charge/item/attempt/ installment deletion is rejected.
- `installment_complete_settlement`: paid installments cannot change; customer
  components cannot remain offered/applied when the installment becomes paid.
  Customer cash-paid components and consumed credits cannot commit separately
  from full installment settlement. Merchant accounting does not require a
  customer cash payment. An unfrozen positive debit may be waived as an economic
  adjustment without pretending that the customer paid it.
- `installment_payment_freeze`: attempts and their economic identity are immutable;
  a finalized snapshot cannot change while financial outcome is unresolved.
  New attempts/items serialize on the installment. Deferred completeness checks
  ensure a snapshot includes principal, upfront, all customer payable charges,
  correct source identities/currency/effects and its aggregate. No charge-only
  snapshot can commit. Items can only be inserted in their attempt's creation
  transaction, and can never be updated/deleted.

These invariants use eight row triggers:

- `installment_no_charges_after_paid` on `installment_charges`, before insert/update/delete.
- `installment_complete_settlement` on `installments`, before insert/update/delete.
- `installment_payment_freeze` on `payment_attempts`, before insert/update/delete.
- `installment_snapshot_immutable` on `payment_attempt_items`, before insert/update/delete.
- `installment_paid_evidence` on `installments`, after insert/update, deferred.
- `installment_snapshot_complete` on `payment_attempts`, after insert, deferred.
- `installment_items_complete` on `payment_attempt_items`, after insert, deferred.
- `installment_components_settled` on `installment_charges`, after update, deferred.

All objects are schema-qualified under `bnpl`. Functions are
`guard_installment_charge`, `guard_installment_settlement`,
`guard_payment_attempt_freeze`, `guard_payment_items`, and
`verify_payment_snapshot_commit`. Their language, security mode, search path,
volatility, signature and source-body SHA-256 are part of the managed contract.

## Settlement sequence and economic meaning

A positive-net obligation creates one immutable attempt/items set, then sends the
aggregate through existing PrepareCollectionObligation/CreateCheckoutPayment.
No external network operation runs inside a database transaction. A trusted FC
result is checked against the local snapshot, identity, currency, full amount
and effective time before monetary application.

Inside one transaction the application finalizes customer components before
MarkPaid, then applies principal-only exposure/remaining-principal changes and
attempt/Inbox/Outbox/audit/journal effects. Customer debits become `paid` with the
FC effective paid timestamp. Discount/waiver credits become `waived` (consumed),
with no false cash PaidAt. First-installment activation follows the same order.
Original installment TotalAmountMinor is not confused with adjusted cash paid.
A paid transition also requires matching full snapshot evidence and a succeeded
attempt at commit; raw partial amounts and paid inserts without evidence fail.

The deferred charge guard permits these temporary intermediate states only
within the full successful transaction; a standalone partial component settlement
rolls back. An attempt-backed settlement must have a matching succeeded attempt
at commit. Replays do not release principal twice. Completion also requires the
entire required installment count, paid required installments, no remaining
principal/customer debit charges and no blocking attempt.

The database validates persisted integrity, not FC ledger truth or pricing.
Its signed snapshot sum is a consistency check, not a second fee engine.
There are no application session flags that bypass guards. A durable
`financial_outcome_resolved` fact distinguishes authoritative failure/cancellation
from a locally terminal-looking state. Only authoritative terminal repository
paths set it. Old records default to unresolved: classify/remediate them using
FC evidence rather than guessing or rewriting their historical outcomes.
Manual review remains financially ambiguous; a late definitive success can apply.
Lookup `not_found` retains reconciliation/freeze because absence is not cancellation.

## Concurrency contract

Normal application order is customer profile, contract, installment parent(s),
PaymentAttempt, InstallmentCharge, then CreditAccount. All coordinated operations
for a customer use the same profile lock. Cancellation retains that boundary;
accepted pre-existing financial operations are honored during finalization.

Direct UPDATE SQL can lock a child before its row trigger executes. Such a trigger
uses parent `FOR UPDATE NOWAIT`; it returns a semantic conflict when the parent
is busy instead of waiting in a child-to-parent deadlock. Attempt establishment
uses the same parent boundary. `financial_revision` is an internal parent counter:
charge mutations/new attempts advance it, making old repeatable-read snapshots
fail serialization rather than silently missing a concurrently committed child.
`snapshot_xid` binds item insertion to the actual creation transaction, including
savepoints; updating an old attempt cannot reopen its immutable item set.

Conflicts may wait or fail/retry; they cannot commit inconsistent obligations.
The generic postgres adapter still maps genuine deadlock/serialization conflicts
normally. No broad exception is reclassified as an invariant violation.

## Semantic failures

Stable private SQLSTATE + constraint pairs map to existing `cerr.CodeConflict`:

- `BN001` / `installment_already_paid`: `error.installment_already_paid`.
- `BN002` / `installment_has_unsettled_charges`: `error.installment_has_unsettled_charges`.
- `BN003` / `installment_payment_in_progress`: `error.installment_payment_in_progress`.
- `BN004` / `installment_financial_component_immutable`: `error.installment_financial_component_immutable`.

The exact pair must match. Raw pgconn diagnostics remain private causes; HTTP
receives localized EN/FA messages and semantic violations, never SQL or function
names. Investigate correlation/audit IDs privately; do not expose diagnostics to
customers or waive a constraint to make a retry pass.

## Startup verification and inspection

NewApp verifies after acquiring its pool and before constructing external clients,
Use Cases, listeners, consumers or workers. Failure goes through existing Close
rollback. Verification is a repeatable-read, read-only transaction and works
with SELECT/schema-USAGE privileges alone. It requires no DDL permissions.
It never migrates, creates, replaces, enables, disables or drops objects.

The verifier checks the required clean migration/checksum, internal schema fields,
trigger schema/table/name/event/timing/enabled state/arguments/condition/deferral,
referenced function schema/name/signature/security/language/search path/volatility,
and function body hashes. It compares stored prosrc, not PostgreSQL-version-sensitive
pretty-printed function DDL. Missing, disabled, wrong-function/event or modified
bodies fail startup with the invariant and action to investigate. Any dirty
migration chain also blocks startup.

Inspect actual objects with ordinary read-only catalog queries:

```sql
SELECT n.nspname,c.relname,t.tgname,t.tgenabled,
       pg_get_triggerdef(t.oid),pg_get_functiondef(t.tgfoid)
FROM pg_catalog.pg_trigger t
JOIN pg_catalog.pg_class c ON c.oid=t.tgrelid
JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='bnpl' AND NOT t.tgisinternal
ORDER BY c.relname,t.tgname;
```

Runtime roles should have no ownership/DDL/TRUNCATE/superuser capabilities.
Administrative disabling/replacing objects is a privileged action, not a business
escape hatch; drift is detected, not silently repaired. Migration execution uses
the project's existing controlled migration credentials/lifecycle separately.

## Developer commands and tests

```sh
make db-invariants-list
make db-invariants-verify                 # CONFIG defaults to configs/dbadmin.yaml
make db-invariants-verify CONFIG=/path/to/config.yaml
COLLECTIONS_TEST_DATABASE_URL='<disposable PostgreSQL>' make -e db-invariants-test
```

The existing Makefile imports `.env`. For an explicitly supplied isolated
configuration environment, use `make -e db-invariants-verify` (or command-line
variable overrides) so that `.env` cannot select another database. Listing needs
no live database/config. Verification never runs migrations. Test target uses
`-count=1 -race` with development/integration tags and fails if PostgreSQL/DSN is
unavailable; it never reports skipped tests as success.

Tests install the complete initial schema into unique schemas in a disposable
database; incremental migrations are exercised separately. They test direct mutations, both competing lock
orders, old repeatable-read snapshots, deferred rollback, read-only verification,
real payment/reconciliation/activation paths, exactly-once principal release,
contract completion, history, Collections and message suppression. Only external
FC calls are faked; repositories and transaction constraints are real.

## Preflight and rollout

The standalone historical-data audit SQL files have been removed. For an
existing database containing retained financial data, operators must perform
an explicit read-only reconciliation assessment against Financial Core before
migration; initial schema verification only checks managed object definitions
and permissions, not the economic correctness of every historical row.

The original pre-release migration contained a historical-data preflight for
installing these guards onto an already populated database. That migration has
now been folded into the initial schema, which creates the protections before
any business rows exist. Fresh initialization neither audits nor rewrites old
financial history because there is none. An existing non-disposable database
must be reviewed and migrated explicitly rather than silently treated as a
fresh initial schema.

For the first Production release, initialize an empty schema using the matching
DB Admin binary and run read-only invariant and privilege verification before
starting Runtime. Do not let an old binary overlap with newly restricted grants.
The integration tests retain a focused read-only query that detects an
inconsistent historical paid installment without modifying evidence.

Future changes require a new incremental migration, explicit registry/checksum
and definition-contract updates, compatibility review and updated real PostgreSQL
tests. Never edit a published baseline/previous migration or disable a guard as
an application workaround.

## Noncash and negative obligations

Core explicitly classifies positive, zero, negative and invalid cash economics.
Positive values use aggregate collection. New incompatible financed zero/negative
or invalid offers/contracts are rejected. Existing such obligations remain unpaid
and retain exposure/financial history; they are quarantined from automatic new
credit, default-entry triggers and false reminders/notices. An existing episode
cannot falsely recover while due economic resolution remains pending.

The listing keeps original `total_amount_minor` and adds nullable
`customer_payable_amount_minor`: a valid zero net is 0; negative/invalid cash is
unknown/null. `next_payable` has false PayableAmountKnown and a precise blocker:
`noncash_settlement_required`, `negative_net_adjustment_required`, or
`financial_adjustment_required`. No partial/second-payment route is advertised.

The checked-in generic `ReverseMerchantEntitlement` RPC exists; its economics
include reversals of receivable, merchant payable, collected cash and merchant
transaction fee. Its existence alone does not establish a funded customer
principal write-off while preserving the merchant's valid payable. This BNPL
integration has no confirmed noncash application/proof contract for that case.
It does not guess reversal counterparties, reverse a merchant payable to force
balance, send a zero/fake checkout or invent a protobuf. Confirm the existing
generic reversal/noncash semantics and funding allocation with FC, then apply
only on authoritative confirmation. Until then the implemented safe lifecycle
is reject new incompatible issuance, inventory/quarantine existing cases and
explicitly reconcile; no automatic principal release or fabricated paid status.
