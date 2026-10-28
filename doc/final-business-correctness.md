# Final BNPL business correctness implementation

Baseline: `10c2b180ba52de1fd71ad7e07488bce05a9b4ce8` on
`feat-add-decision-domain`. Implemented and verified on 2026-10-28.
No PR, merge to main, gateway, Envoy, DNS, TLS or infrastructure-repository change.

## Payment authority and component accounting

Financial Core remains product-agnostic. BNPL computes and retains the breakdown;
FC receives the total using the already integrated collection/checkout protobufs.
No additional protobuf or component-aware Financial Core API is required or added.

Persisted principal, upfront, BNPL fee, commission, late fee and tax increase the
collection; customer discounts and waivers decrease it. Merchant-paid charges do
not enter the customer's collection. Component identity, payer, recipient,
currency, status, positive item amounts and arithmetic are validated before
snapshotting. A fee and discount that net to principal retain distinct items.
The previous principal-only initiation, preparation and summary guards are removed.

1. The command locks customer and contract, enforces sequencing/global priority,
   and creates an idempotent attempt with immutable component items and audit.
2. After commit, existing `PrepareCollectionObligation` receives the net amount,
   source identity and existing merchant-entitlement reference. Its returned
   outstanding amount and currency must match. Existing `CreateCheckoutPayment`
   receives that same amount. Existing response validation and session TTL apply.
3. Incoming FC events wake the consumer. Existing `GetPaymentStatus` supplies the
   authoritative identity, status, aggregate amount, currency and effective time.
   Browser redirects, session creation and local attempt rows prove no settlement.
4. Under the customer lock, BNPL compares the immutable snapshot with all current
   collectible charge rows. Missing/new/changed components reconcile instead of
   silently disappearing. It then applies installment settlement, all snapshotted
   customer charge effects (including consumed discounts/waivers), principal-only
   exposure release, attempt, Inbox, audit and Outbox atomically.
5. Reconciliation applies the same rules. Duplicate/inbox replay/out-of-order
   results do not release exposure twice. Amount and currency are checked on
   successful replay too. Completion also requires all required installments
   paid, no remaining principal, no unpaid customer debit charges and no blocking
   payment attempt.

Historical principal-paid rows with unpaid customer debit charges remain debt,
including rows on completed contracts. Existing list/count/due/priority queries
and the command permit a residual-charge-only attempt. That attempt includes no
principal or historical upfront amount. Application preserves the original
installment payment amount and effective time; the additional payment lives in
its own attempt/items. It never releases principal again. Such debt also remains
visible to Collections and default-notification candidate selection.

The repayment summary advertises the real positive aggregate. It also includes
`outstanding_amount_minor` and an optional `payment_block_reason`. Nonpositive or
invalid net collections are never fabricated as successful cash payments;
`financial_adjustment_required` identifies this exceptional economic state.
See [the existing-proto collection contract](financial-core/aggregate-collection.md).

## Calendar grace and historical policy selection

`collections_default:v2` uses the injected business location (Iran configuration:
`Asia/Tehran`). The due calendar date is D0. D+1 through D+N remain within grace;
default starts at local 00:00 on D+N+1. Zero grace means the midnight after D0.
A non-midnight DueAt does not shorten grace. Persisted values above 31 remain
valid; there is no 31-day clamp. Calendar arithmetic handles DST.

Bootstrap explicitly supplies a small Collections registry. `v1` preserves its
previous elapsed-UTC-day boundary and stays resolvable for immutable history.
New episodes use v2. Recovery intentionally resolves the opening Decision's
policy ID/version, retains its lineage, and rejects unknown historical versions.
An independent later episode starts a fresh Decision chain. Overdue lifecycle,
notification timing and PostgreSQL calendar selectors use business timezone;
pool runtime timezone is supplied from the existing clock configuration.

## Default notice

The separate pure notification policy permits a request at/after 13:00 on the
first default calendar date. Delayed runs catch up on subsequent business days
after 13:00. It requires an active episode, still-unpaid original trigger debt
and no payment/cancellation uncertainty. Recovery before dispatch suppresses it.

The episode reserves reminder precedence before 13:00 and during uncertainty.
Live default eligibility also closes the scheduler gap before an episode exists.
Ordinary reminders cannot consume that reserved slot. The existing customer/day
ceiling remains effective, including across two episodes on the same day.
`collections-default:<episode UUID>` is the durable notification idempotency key.
A second independent episode may request a second notice on an allowed day.

Customer-lock revalidation, existing ShortLink, Reminder and SMS Outbox writes
share a transaction. Outbox failure rolls back the link/reminder, enabling safe
retry. Concurrent runs create one durable request per episode, not a daily repeat.
Template: `bnpl.collections.default.v1`; notification policy version:
`collections_default_notification.v1`. A request is not proof of SMS delivery.

## Cancellation, concurrency and recovery

Outstanding obligations include `canceling` contracts. Pending/ambiguous
cancellation is not settlement, cannot falsely release default restrictions,
and blocks new credit. The command still disallows starting repayment while
canceling. Definitive FC rejection compensates to active debt; definitive
acceptance follows the existing cancellation monetary/exposure application.

Customer profile is the shared serialization boundary. Payment-result and
reconciliation application acquire customer then contract before
Inbox/attempt/obligation/charge/account mutations. Cancellation Saga business
steps use the same customer-before-contract order. New-credit guards already
run inside the customer lock. Checkout-result finalization, direct activation
and merchant-funding finalization now share that boundary as well. FC network
operations remain outside the database transaction. Finalization honors financial
operations accepted before eligibility changed; a newly initiated obligation is
checked by the existing live guard before its FC operation.

Effective FC settlement time is distinct from receipt/persistence time. A late
arrival that proves settlement before the default boundary emits corrective
recovery evidence (`late_authoritative_settlement_correction`) while retaining
original Decisions, triggers and episode/case history.

After a settlement transaction commits, the result/reconciliation consumer
reassesses the customer. Hook failure returns an error to the existing durable
consumer; Inbox replay retries reassessment. The periodic task remains the safety
net, including crash/retry exhaustion. Production config requires assessment and
reminder dispatch enabled. Future unpaid installments do not block recovery.
Recovery selects the active restriction by its episode and checks the opening
Decision; other reasons remain active. CollectionsCase/legal proceedings are not
automatically closed by credit recovery.

## Existing customer-credit endpoint

`GET /api/v1/customer-credit` retains its existing envelope/meta convention,
account lifecycle, currency and available-amount behavior, and adds:

```json
{"credit_usage":{"status":"restricted","reason":"collections_default"}}
```

Statuses are `allowed`, `restricted`, `unavailable`. Independent or sensitive
restrictions expose only `credit_usage_restricted`; no investigation details are
leaked. Active restrictions/episodes and live policy eligibility cover scheduler
delay. Not-assigned/expired summaries remain distinct. No credit capacity or
exposure is rewritten to represent a restriction. Swagger and serialization
coverage are updated; no new endpoint is introduced.

## Persistence and history

The frozen baseline and general `decisions` table, columns, indexes, constraints
and accepted values are unchanged. Two incremental specialized migrations:

- `000001_collections_default_notification.sql`: extend reminder type constraint.
- `000002_independent_credit_restrictions.sql`: canonical independent reason
  values; active uniqueness becomes customer/type/reason instead of customer/type.

Apply these before deploying binaries that write the new reminder type or
independent reasons. Existing rows are retained. No migration deletes or rewrites
business rows. Domain history is preserved: release restrictions, recover episodes,
append Decisions/events, and retain payments, reminders and links.

## Test architecture and verification

The former combined lifecycle test is split into independently named cases with
shared small fixtures and one unique PostgreSQL schema per test. No ordering or
cross-test history dependency, broad cleanup or silent unavailable-DB skip.
Contexts are bounded and clocks deterministic. Monetary-path tests use actual
repositories, Inbox/Outbox, payment-result/reconciliation code, exposure mutations
and recovery; only the external FC client is faked. Focused repository/notification
fixtures may set current state directly and are labeled accordingly.

Coverage includes calendar/DST/Tehran and long grace; v1 historical recovery;
concurrent default entry, payment application and credit guard; authoritative
cancellation accepted/rejected steps; ambiguous payments; lineage/history;
rollback; default notice timing, catch-up, daily ceiling, separate episodes,
concurrent dispatch and Outbox retry; read snapshots; aggregate fees/tax/discount/
waiver settlement; residual-only command creation/replay and application on active
and completed contracts; no repeated principal release; mismatch reconciliation;
public summary serialization; and invalid injected identifiers.

Verification commands (all executed successfully; logs retained outside Git):

```sh
make swagger
go test -tags development ./...
COLLECTIONS_TEST_DATABASE_URL='<disposable database>' go test -race -tags 'development integration' ./pkg/core/usecase/collectionsuc/...
go test -tags production ./cmd/bnpl/... -run '^$'
git diff --check
```

Formatting uses goimports/gofmt. `go test` includes its default vet checks.
The integration database is the task-owned disposable PostgreSQL on local port
55439, separate from the shared database. No live FC or SMS-provider E2E call
was made; tests verify the existing port contracts, not external deployment state.

## Deployment assumptions and limits

The existing FC aggregate/idempotency contract remains authoritative; no
fee-specific external API dependency remains. The Notification deployment must
resolve template `bnpl.collections.default.v1` through its existing template
mechanism. Timing is scheduler-granular: 13:00 is eligibility, not a promise of
delivery at that instant. Production needs both required tasks and the existing
consumer retry/Outbox workers running.

Zero/negative-net adjustment is explicitly not treated as a cash checkout. No
financial write-off or noncash settlement is invented. Historical inconsistent
snapshots/new charges reconcile for deliberate correction rather than being
silently paid. The residual-charge path requires the existing contract's FC
entitlement identity; missing historical financial identity stays a visible
reconciliation problem instead of routing money to a guessed account.

## Exact changed files

The following list is relative to the repository root and includes generated
mocks and Swagger artifacts.

- `.env.example`
- `assets/db/migrations/000001_collections_default_notification.sql`
- `assets/db/migrations/000002_independent_credit_restrictions.sql`
- `cmd/bnpl/bootstrap/usecase.go`
- `doc/collections-correctness-verification.md`
- `doc/final-business-correctness.md`
- `doc/financial-core/aggregate-collection.md`
- `gen/swagger/docs.go`
- `gen/swagger/swagger.json`
- `gen/swagger/swagger.yaml`
- `pkg/adapter/config/database.go`
- `pkg/adapter/config/load.go`
- `pkg/adapter/config/usecase.go`
- `pkg/adapter/config/usecase_test.go`
- `pkg/adapter/config/validate.go`
- `pkg/adapter/config/validate_test.go`
- `pkg/adapter/db/postgres/customerrestrictionrp/query.go`
- `pkg/adapter/db/postgres/customerrestrictionrp/repo.go`
- `pkg/adapter/db/postgres/installmentchargerp/query.go`
- `pkg/adapter/db/postgres/installmentchargerp/repo.go`
- `pkg/adapter/db/postgres/installmentrp/query.go`
- `pkg/adapter/db/postgres/repaymentreminderrp/query.go`
- `pkg/adapter/db/postgres/repaymentreminderrp/repo.go`
- `pkg/adapter/restful/gin/customercreditrs/serdser.go`
- `pkg/adapter/restful/gin/customercreditrs/serdser_test.go`
- `pkg/adapter/restful/gin/customerrs/list_unpaid_installments.go`
- `pkg/core/mock/mock_repo_CustomerRestrictionTxQueryer.go`
- `pkg/core/mock/mock_repo_InstallmentChargeTxQueryer.go`
- `pkg/core/mock/mock_repo_RepaymentReminderConnQueryer.go`
- `pkg/core/mock/mock_repo_RepaymentReminderQueryer.go`
- `pkg/core/mock/mock_repo_RepaymentReminderTxQueryer.go`
- `pkg/core/model/collection_components.go`
- `pkg/core/model/collection_components_test.go`
- `pkg/core/model/customer_restriction.go`
- `pkg/core/model/installment.go`
- `pkg/core/model/installment_charge.go`
- `pkg/core/model/installment_grace_test.go`
- `pkg/core/model/repayment_reminder.go`
- `pkg/core/policy/collections/policy.go`
- `pkg/core/policy/collections/v1/policy.go`
- `pkg/core/policy/collections/v1/policy_test.go`
- `pkg/core/policy/collections/v2/policy.go`
- `pkg/core/policy/collections/v2/policy_test.go`
- `pkg/core/policy/defaultnotification/v1/policy.go`
- `pkg/core/policy/defaultnotification/v1/policy_test.go`
- `pkg/core/policy/repaymentpriority/v1/policy.go`
- `pkg/core/policy/repaymentreminder/v1/policy.go`
- `pkg/core/policy/repaymentreminder/v1/policy_test.go`
- `pkg/core/repo/customer_restrictionrp.go`
- `pkg/core/repo/installment_chargerp.go`
- `pkg/core/repo/installmentrp.go`
- `pkg/core/repo/repayment_reminderrp.go`
- `pkg/core/usecase/collectionsuc/assess.go`
- `pkg/core/usecase/collectionsuc/assess_test.go`
- `pkg/core/usecase/collectionsuc/credit_guard_test.go`
- `pkg/core/usecase/collectionsuc/default_notification.go`
- `pkg/core/usecase/collectionsuc/fingerprint.go`
- `pkg/core/usecase/collectionsuc/fingerprint_test.go`
- `pkg/core/usecase/collectionsuc/identifier_test.go`
- `pkg/core/usecase/collectionsuc/lifecycle_integration_test.go`
- `pkg/core/usecase/collectionsuc/payment_components_integration_test.go`
- `pkg/core/usecase/collectionsuc/usecase.go`
- `pkg/core/usecase/contractuc/activate.go`
- `pkg/core/usecase/contractuc/cancellation_saga.go`
- `pkg/core/usecase/contractuc/create_active.go`
- `pkg/core/usecase/contractuc/customer_synchronization.go`
- `pkg/core/usecase/customeraccountuc/list_unpaid_installments.go`
- `pkg/core/usecase/customeraccountuc/repayment_summary_test.go`
- `pkg/core/usecase/customeraccountuc/types.go`
- `pkg/core/usecase/customercredituc/assessor_test.go`
- `pkg/core/usecase/customercredituc/assign_initial_limit_test.go`
- `pkg/core/usecase/customercredituc/collections_guard.go`
- `pkg/core/usecase/customercredituc/credit_usage_test.go`
- `pkg/core/usecase/customercredituc/get_summary.go`
- `pkg/core/usecase/customercredituc/get_summary_test.go`
- `pkg/core/usecase/customercredituc/types.go`
- `pkg/core/usecase/customercredituc/usecase.go`
- `pkg/core/usecase/financialcorepaymentreconciliationuc/process.go`
- `pkg/core/usecase/financialcorepaymentresultuc/collections_coordination.go`
- `pkg/core/usecase/financialcorepaymentresultuc/installment_success.go`
- `pkg/core/usecase/financialcorepaymentresultuc/process.go`
- `pkg/core/usecase/financialcorepaymentresultuc/reconciliation_apply.go`
- `pkg/core/usecase/financialcorepaymentresultuc/usecase.go`
- `pkg/core/usecase/financialcorepaymentresultuc/validation.go`
- `pkg/core/usecase/installmentlifecycleuc/audit_outbox.go`
- `pkg/core/usecase/installmentlifecycleuc/mark_due_overdue.go`
- `pkg/core/usecase/installmentlifecycleuc/mark_due_overdue_test.go`
- `pkg/core/usecase/installmentrepaymentuc/create_checkout_payment_response_test.go`
- `pkg/core/usecase/installmentrepaymentuc/create_payment_attempt.go`
- `pkg/core/usecase/installmentrepaymentuc/financial_core_payment.go`
- `pkg/core/usecase/installmentrepaymentuc/financial_core_payment_test.go`
- `pkg/core/usecase/repaymentreminderuc/usecase.go`
