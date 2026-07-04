# Atomic installment settlement and managed database invariants

Implementation baseline: `0c79f1ceac0b6bd69364eff09e21350ec959f7fa`.
Branch: `feat-add-decision-domain`. No PR or merge into main.

## Business result

A paid installment is fully closed and terminal. There is one complete positive
cash collection, with an immutable local component snapshot and authoritative
FC result. Principal-only closure followed by residual/charge-only collection is
removed from Core, repositories, Collections, reminders, summaries and tests.
Paid rows never appear in regular unpaid queries, including historical paid rows
with outstanding charges. The mapper continues to accept only valid active
installments; the old listing/internal-error mismatch is eliminated at selection.

Historical invalid states are not repaired or deleted automatically. Read-only
SQL inventories identify customer, contract, installment, charge and attempt IDs.
Migration preflight refuses incompatible records with actionable diagnostics and
rolls back instead of mutating history. Tests intentionally construct old invalid
rows before installation to prove audit visibility and failure without repair.

## Complete cash payment flow

1. Serialize customer/contract/installment; validate ownership, sequence, global
   repayment priority and persisted component identities/economics.
2. Classify positive, zero, negative or invalid net cash. Only positive valid
   cash creates an attempt. Create the immutable complete item set atomically.
3. Outside DB transactions, use the existing generic PrepareCollectionObligation
   and CreateCheckoutPayment with one aggregate. Validate returned amount,
   currency, financial identity and action/session response.
4. On a wake-up event, the existing GetPaymentStatus port supplies authoritative
   FC status, aggregate, currency and effective time. No browser or local attempt
   is treated as settlement.
5. Under parent-before-child locks validate the full immutable snapshot against
   current components and exact FC result. Reject partial/mismatched amounts.
6. Within one transaction finalize customer debits as paid, consume discount/
   waiver credits as waived without a cash PaidAt, then MarkPaid. Apply only
   principal to exposure/remaining principal. Persist attempt, Inbox, Outbox,
   audit and credit-journal effects together. Deferred guards check the complete
   successful snapshot and installment at commit.
7. Only after commit reassess Collections; existing durable retry and periodic
   safety nets remain. Replays and concurrent duplicate events do not repeat
   principal release. Contract completion also checks the expected installment
   count, all required installments paid, remaining principal zero, no unpaid
   customer debit and no blocking attempt.

First-installment activation uses the same component-before-parent order.
The old upfront/principal-only checkout amount restriction is removed: aggregate
fees and credits are supported using existing FC APIs. Original contractual total
can differ from actual cash. Replay proves the immutable commitment/paid amount,
not equality with the original total. The direct trusted activation wrapper also
records successful attempt application in its own transaction; consumer use still
provides the normal durable Inbox/Outbox boundary.

## Database guarantees and snapshot freeze

The three managed domain rules use eight triggers and five functions, detailed
in [the architecture guide](architecture/database-invariants.md). Migration:
`assets/db/migrations/000003_installment_payment_invariants.sql`.

Paid installments cannot accept new charges, economic or metadata changes,
ownership moves, deletion or state regression. Customer components must be
finalized before MarkPaid. A paid transition must match a complete immutable
principal-containing payment snapshot and succeeded cash evidence at commit;
raw partial amounts, paid inserts without evidence and charge-only snapshots fail.
Cash-paid components/consumed credits cannot commit as a partial settlement while
the installment stays unpaid. Merchant accounting is not charged to the customer.

Attempt creation and every financial mutation share the installment parent lock.
The snapshot is frozen for created, submitted, reconciliation and manual-review
states, success pending application, and terminal-looking states whose financial
outcome remains unproven. Neither a locally canceled row nor a not-found lookup
proves cancellation of an in-flight external operation. Only authoritative
terminal failure/cancellation repository paths set `financial_outcome_resolved`.
Old rows conservatively default false and require explicit evidence-based review.
Late authoritative success after manual review can still apply.

`financial_revision` advances on child economics/attempt establishment so an old
repeatable-read transaction cannot miss a committed child mutation. `snapshot_xid`
binds item insertion to the actual attempt-creation transaction; old item sets
cannot be reopened by updating their parent. These are internal integrity facts,
not configurable bypasses or financial ledger truth.

## Locking and errors

Application order: CustomerProfile, Contract, Installment parent(s), PaymentAttempt,
InstallmentCharge, CreditAccount. Collections/cancellation share the customer
boundary. Direct SQL UPDATE may lock a child first; trigger parent locking uses
NOWAIT to produce a bounded semantic conflict instead of a reverse-order deadlock.
Separate PostgreSQL sessions test both orders and deliberately observed lock waits.

Exact SQLSTATE/constraint pairs map privately to existing cerr Conflict semantics:
BN001/already-paid, BN002/unsettled-or-incomplete settlement,
BN003/payment-in-progress, BN004/immutable financial component. EN/FA messages and
HTTP conflict tests verify no SQL/function diagnostics leak. Unknown exceptions
or similarly named constraints retain normal internal-error handling.

## Managed registry, startup and tooling

`pkg/adapter/db/postgres/invariants` contains discoverable structured metadata,
source-body hashes, migration checksum, read-only catalog/schema verification and
tests. Startup invokes verification immediately after opening the normal-role
pool, before external clients/Use Cases/traffic/workers. Existing App.Close rollback
owns cleanup on failure. Startup never runs migrations or DDL.

Checks include object/schema/table/event/timing/enabled/condition/argument/deferral,
referenced function identity/language/signature/security/search path/volatility,
source body, required internal columns and clean compatible migration metadata.
A role granted only schema-USAGE and SELECT successfully verifies; its DDL attempt
is denied. Missing, disabled, wrong-function, wrong-event and edited-body cases
fail verification; missing/incompatible migration history is also detected.

Developer commands are `db-invariants-list`, `db-invariants-verify` and
`db-invariants-test`. List is offline. Verify uses existing configuration and is
read-only. Test requires real disposable PostgreSQL and uses -count=1/-race, never
silent skips. The Makefile's existing .env import requires -e or explicit variable
overrides when selecting a temporary configuration environment.

## Zero, negative and invalid economics

Core explicitly classifies all four cases. Positive cash follows the full-payment
flow. New incompatible financed zero/negative or invalid schedules are rejected
at offer presentation/contract acceptance, before FC submission. Existing cases
stay unpaid with exposure/history intact, block new credit and are excluded from
misleading automated default triggers and payment/default notices. Due unresolved
adjustments do not falsely recover an existing episode. Read-only inventory and
explicit FC/reconciliation review are required; no zero/fake cash payment or local
principal write-off is fabricated.

The generic ReverseMerchantEntitlement RPC is present and was inspected, including
its receivable/merchant-payable/cash/merchant-fee reversal economics. The current
integration does not establish safe funded noncash principal settlement while
preserving the merchant's valid payable. It must not guess who funds a discount,
reverse merchant payable to force balance, or declare a prepare response a
settlement (that RPC creates an instrument, not accounting application). The
remaining dependency is confirmation/support for the appropriate generic noncash
application and its authoritative evidence, not a BNPL-aware fee API or invented
protobuf. Automatic noncash paid/exposure release is intentionally unsupported
until that contract and economic funding are established. The requested safe
fallback lifecycle is implemented rather than only returning a blocker string.

## API and customer communication

Existing endpoints and envelopes remain. Unpaid listing, count, due list and
priority use only valid unpaid rows. Original `total_amount_minor` remains;
nullable `customer_payable_amount_minor` exposes current complete cash, including
zero as 0 and negative/invalid as null. next_payable retains its existing payable
fields and uses precise noncash/negative/invalid reconciliation blockers.
A frozen attempt's amount cannot silently change. Swagger and HTTP serialization
coverage are updated; no partial-payment choice or second-paid-installment route
is added.

Calendar grace is unchanged: D0 due calendar date, all D+1..D+N allowed, default
at local midnight D+N+1, notice from 13:00, historical policy resolution preserved.
If A opened the episode, A settles and B is due but within grace, the episode and
restriction stay active while the overdue-entry message is suppressed. If B later
qualifies as overdue, an unsent episode notice can truthfully reference B, never
paid A. Immutable opening triggers/history remain unchanged; episode dedupe and
daily ceiling prevent repeated messages. Payment/cancellation/economic uncertainty
suppresses false communication.

## Verification, preflight and deployment

Final verification results and the changed-file manifest are recorded below. Only dedicated disposable databases/schemas were used for writes.
No live FC/SMS provider E2E call or production migration was performed.

The CLI installation test uses the frozen initializer and the existing ordered
migration command, followed by the same verifier used at startup. Historical
preflight tests produce expected failures and retain original rows/statuses.
Clean disposable preflight findings do not constitute an audit of a live user DB.

Drain all old writers/consumers/schedulers before migration: old binaries mark
paid before charges and are incompatible with immediate integrity checks. Run
read-only audit, explicitly reconcile findings, apply the versioned migration,
verify, then start the new binary and restore traffic/workers. Table locks and
metadata storage rewrite require a sized maintenance window; there is no unsafe
old/new rolling overlap or startup repair flag.

The decisions table and frozen schema/checksum are unchanged. No previous migration
was edited. No Decision, payment, installment/charge, episode/restriction/case,
reminder or link history is deleted or silently repaired. No FC protobuf, Envoy,
gateway, DNS or TLS change is included. Historical reports are retained with an
explicit superseded notice instead of erasing review history.

## Executed verification — 2026-10-28

All following commands completed successfully on the final implementation:

- `go test -tags development ./...` — all packages passed.
- `go test -count=1 -race -tags 'development integration' ./pkg/core/usecase/collectionsuc/... ./pkg/adapter/db/postgres/invariants/... ./pkg/core/usecase/contractuc` — passed against real PostgreSQL, respectively 45.250s, 44.670s, 9.898s. External FC/SMS ports were fakes; persistence and transactions were real.
- `go test -tags production ./cmd/bnpl/... -run '^$'` — production build passed.
- `make db-invariants-list` — passed offline; listed all eight trigger registrations for the three domain invariants.
- `make -e db-invariants-verify` — passed with the isolated database environment; same read-only verifier as normal startup.
- `COLLECTIONS_TEST_DATABASE_URL=<dedicated-test-DSN> make -e db-invariants-test` — real PostgreSQL tests passed with `-count=1 -race` (11.066s); no dependency skips.
- `make swagger` — generated all three Swagger outputs successfully.
- `git diff --check` — clean.
- Existing `db migrate` CLI applied migrations 000001–000003 transactionally to a newly initialized disposable database. Final migration checksum: `sha256:6267ddaade88869646b835af4f2a8dfc2c4ed4c1c6ce91ff065fa5cf39be18a4`.
- Both read-only audit scripts completed on that clean disposable database: zero findings. Deliberate corrupt-history integration fixtures separately verified diagnostic failure, preserved records and rollback without partial installation. A live/customer database has **not** been audited or migrated.

The dedicated PostgreSQL container was `bnpl-codex-correctness`, port 55439;
application tests used isolated schemas. Migration/CLI checks used a separate
temporary database. The shared project database was not used for migration or
financial test writes. The CLI fixture's initial connection-role mismatch was
corrected before the successful installation; it was a test harness configuration
issue, not a migration change.

No mocks required regeneration because no port signature changed. Integration
fixtures were updated to persist complete attempt/item sets in one transaction,
and to provide explicit successful settlement evidence instead of raw paid rows.

## Changed-file summary and manifest

Changes cover Core economic classification, atomic activation/repayment/result
application, financial ambiguity and Collections communication; PostgreSQL queries,
new versioned guards and verifier; startup/CLI/Make tooling; current-payable API and
localization/Swagger; formerly standalone read-only audits, real PG/concurrency
tests and documentation. The audits were later removed in favor of an inline
integration-test query; this manifest records the original implementation.
The following manifest is relative to the repository root:

- `Makefile`
- `assets/db/migrations/000003_installment_payment_invariants.sql`
- `cmd/bnpl/bootstrap/app.go`
- `cmd/bnpl/bootstrap/boot.go`
- `cmd/bnpl/bootstrap/invariants.go`
- `cmd/bnpl/command/invariants.go`
- `doc/architecture/database-invariants.md`
- `doc/atomic-installment-settlement-verification.md`
- `doc/final-business-correctness.md`
- `doc/financial-core/aggregate-collection.md`
- `gen/swagger/docs.go`
- `gen/swagger/swagger.json`
- `gen/swagger/swagger.yaml`
- `pkg/adapter/db/postgres/error.go`
- `pkg/adapter/db/postgres/installmentchargerp/query.go`
- `pkg/adapter/db/postgres/installmentrp/query.go`
- `pkg/adapter/db/postgres/invariant_error_test.go`
- `pkg/adapter/db/postgres/invariants/concurrency_integration_test.go`
- `pkg/adapter/db/postgres/invariants/errors.go`
- `pkg/adapter/db/postgres/invariants/errors_test.go`
- `pkg/adapter/db/postgres/invariants/metadata.go`
- `pkg/adapter/db/postgres/invariants/registry.go`
- `pkg/adapter/db/postgres/invariants/registry_test.go`
- `pkg/adapter/db/postgres/invariants/verify.go`
- `pkg/adapter/db/postgres/invariants/verify_integration_test.go`
- `pkg/adapter/db/postgres/paymentattemptrp/aggregate.go`
- `pkg/adapter/db/postgres/paymentattemptrp/query.go`
- `pkg/adapter/db/postgres/paymentattemptrp/reconciliation.go`
- `pkg/adapter/restful/gin/customerrs/list_unpaid_installments.go`
- `pkg/adapter/restful/gin/customerrs/repayment_amounts_test.go`
- `pkg/adapter/restful/gin/serdser/invariant_error_test.go`
- `pkg/adapter/restful/i18n/builtin/messages/en-US.yaml`
- `pkg/adapter/restful/i18n/builtin/messages/fa-IR.yaml`
- `pkg/core/model/collection_components.go`
- `pkg/core/model/collection_components_test.go`
- `pkg/core/model/installment_charge.go`
- `pkg/core/model/payment_attempt.go`
- `pkg/core/model/payment_attempt_item.go`
- `pkg/core/model/payment_attempt_test.go`
- `pkg/core/policy/collections/policy.go`
- `pkg/core/policy/repaymentpriority/v1/policy.go`
- `pkg/core/repo/installmentrp.go`
- `pkg/core/usecase/checkoutsessionuc/cancel.go`
- `pkg/core/usecase/collectionsuc/assess.go`
- `pkg/core/usecase/collectionsuc/default_notification.go`
- `pkg/core/usecase/collectionsuc/lifecycle_integration_test.go`
- `pkg/core/usecase/collectionsuc/payment_components_integration_test.go`
- `pkg/core/usecase/contractuc/activate.go`
- `pkg/core/usecase/contractuc/atomic_activation_integration_test.go`
- `pkg/core/usecase/contractuc/cancel_test.go`
- `pkg/core/usecase/contractuc/cash_obligations.go`
- `pkg/core/usecase/contractuc/checkout_activation_amount_test.go`
- `pkg/core/usecase/contractuc/checkout_payment_session.go`
- `pkg/core/usecase/contractuc/checkout_payment_session_recovery_test.go`
- `pkg/core/usecase/contractuc/checkout_payment_session_test.go`
- `pkg/core/usecase/contractuc/create_active.go`
- `pkg/core/usecase/contractuc/create_payment_pending.go`
- `pkg/core/usecase/contractuc/expire_payment_pending.go`
- `pkg/core/usecase/contractuc/expire_payment_pending_test.go`
- `pkg/core/usecase/contractuc/origination_test.go`
- `pkg/core/usecase/contractuc/payment_pending_expiry_safety_test.go`
- `pkg/core/usecase/contractuc/submit_prepared_checkout_fulfillment_test.go`
- `pkg/core/usecase/contractuc/usecase_test.go`
- `pkg/core/usecase/customeraccountuc/list_unpaid_installments.go`
- `pkg/core/usecase/customeraccountuc/list_unpaid_installments_test.go`
- `pkg/core/usecase/customeraccountuc/types.go`
- `pkg/core/usecase/customercredituc/collections_guard.go`
- `pkg/core/usecase/decisionuc/installments.go`
- `pkg/core/usecase/financialcorepaymentreconciliationuc/process.go`
- `pkg/core/usecase/financialcorepaymentresultuc/collections_coordination.go`
- `pkg/core/usecase/financialcorepaymentresultuc/installment_failed.go`
- `pkg/core/usecase/financialcorepaymentresultuc/installment_success.go`
- `pkg/core/usecase/financialcorepaymentresultuc/process.go`
- `pkg/core/usecase/financialcorepaymentresultuc/reconciliation_apply.go`
- `pkg/core/usecase/financialcorepaymentresultuc/validation.go`
- `pkg/core/usecase/installmentrepaymentuc/create_payment_attempt.go`
- `pkg/core/usecase/installmentrepaymentuc/financial_core_payment.go`
