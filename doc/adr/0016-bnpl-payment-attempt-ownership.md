# ADR 0016: BNPL PaymentAttempt vs Financial Core payment ownership

## Status

Accepted

## Context

Later installment repayments require local orchestration, idempotency, and projection of Financial Core results without duplicating provider payment-attempt lifecycle inside BNPL.

## Decision

1. **Financial Core** remains authoritative for payment intents, provider attempts, authorization, capture, settlement, refund, reversal, and monetary truth.

2. **BNPL `PaymentAttempt`** is a local orchestration aggregate keyed by contract/installment with states `created → submitted → succeeded|failed|reconciliation_required|canceled`. One BNPL attempt may map to multiple provider attempts inside Financial Core.

3. **`Inbox`** (`integration_inbox` table) provides durable idempotency for inbound Financial Core events. Kafka messages are acknowledged only after inbox (and domain) state commits.

4. **`integration_outbox`** publishes `bnpl.payment_application_result.v1` business acknowledgements (`applied`, `already_applied`, `reconciliation_required`), not transport ACKs.

5. **`financialcorepaymentresultuc`** owns all inbound Financial Core payment-result processing (inbox acquire, routing by `payment_purpose`, outbox acknowledgements). Checkout activation domain logic remains in `contractuc.ApplyCheckoutPaymentActivationInTx`.

6. Installment concerns split: `decisionuc` (checkout installment offer creation on approved/partial Decisions), `installmentlifecycleuc` (expiry and due/overdue jobs), `installmentrepaymentuc` (payment attempt creation with atomic `PaymentAttempt` + item snapshots).

7. **`PaymentAttemptItem` is a child entity of the `PaymentAttempt` aggregate** — persisted in `payment_attempt_items` but managed only through `PaymentAttempt` repository methods (`CreateWithItems`, `LockByIDWithItems`, etc.). No independent use case, HTTP resource, or lifecycle.

8. Snapshots capture **principal only** (`PrincipalAmountMinor`) plus **customer-payable** charges as separate debit/credit lines. Merchant/platform-paid charges are excluded. Attempt total = sum(debits) − sum(credits).

9. Successful payment application marks **only snapshotted customer debit charge** rows paid (by source charge IDs). Credit items (discount/waiver) are never marked paid.

10. **`ProcessResult`** (process-level) is separate from **`paymentapplication.ApplicationResult`** (outbox acknowledgement). Failed/canceled Financial Core results return `failure_recorded` / `cancellation_recorded` — never publish `applied`.

11. Inbox fingerprint is **canonical** (typed business fields hashed deterministically), not raw JSON bytes. Payload identity conflicts persist in `integration_inbox_conflicts` without mutating terminal inbox rows.

12. **Financial Core `financial_payment_intent_id`** is the primary correlation key; provider attempt ID is optional and used for audit when supplied. A provider-attempt mismatch alone does not reject a matching intent.

13. Installment payment adapter remains **noop** until the official Financial Core gRPC contract is available.

14. Outbound **`bnpl.payment_application_result.v1`** is a business acknowledgement envelope (`paymentapplication.Event`) published only for inbound **succeeded** payments. Outbox Kafka `message_key` = `financial_payment_intent_id`. Delivery is at-least-once; Financial Core deduplicates by outbound `event_id`. `event_already_processed` is distinct from payment-application `already_applied`.

15. Kafka adapter requires trusted envelope fields (`source_service`, `event_id`, `event_type`, `event_version`); temporary compatibility defaults are isolated in one adapter mapper and logged when applied. Payment status and business fields are validated **after** inbox acquisition. Unsupported statuses durably fail inbox without outbound acknowledgement.

16. Integration identity fields are normalized (trim whitespace) before inbox insert, fingerprint, and outbound events — stored values match hashed values.

17. Raw unknown payment statuses are preserved in `RawPaymentStatus` and canonical fingerprints so distinct unknown statuses cannot share a fingerprint.

18. Successful payments referencing missing BNPL contract/installment become `reconciliation_required` (`contract_not_found`, `installment_not_found`) — not infinite Kafka retry.

19. Checkout and installment reconciliation persistence are separate functions; checkout path does not accept a payment-attempt repository.

20. Optional provider-attempt IDs persist as SQL `NULL`. Installment `already_applied` is evaluated before current exposure/principal checks.

21. `contractuc.ReconciliationError` provides typed checkout reconciliation reasons; exact first-installment amount is a domain invariant in `contractuc`.

22. Fingerprint generator output is validated before persistence. Legacy raw development fingerprints require database reset or rewrite before replay.

23. **`pkg/core/event/paymentapplication`** holds the pure payment-application business-event contract. **`pkg/core/fingerprint`** defines the minimal hashing port (`Generate` only); SHA-256 is implemented in `pkg/adapter/infrastructure/fingerprint/sha256`. Use cases validate `Value` before persistence. Fingerprints persist as `sha256:<digest>` and are not runtime-configurable. No panic-based fingerprint helpers in Core.

24. Succeeded payments without `financial_payment_intent_id` are permanently rejected after inbox acquisition (`missing_payment_intent`, `permanently_rejected`); no payment-application outbox is created.

25. Installment `already_applied` requires paid installment and compatible contract state — not only succeeded `PaymentAttempt`. Recovery uses `Installment.PaidAt` as `BNPLAppliedAt`, not acknowledgement time.

26. Checkout stores final successful provider attempt in `contracts.successful_financial_payment_attempt_id`; `financial_payment_attempt_id` retains the checkout-session attempt.

27. Known payment statuses are canonicalized to typed values before fingerprinting; unknown statuses remain distinct after normalization.

28. First-installment checkout activation locks and validates charge rows, then marks only customer debit charges paid via `MarkSnapshottedChargeIDsPaid`. `MarkReconciliationRequired` transition conflicts are classified from locked attempt state.

## Consequences

- Reconciliation-required payments are never silently discarded at the consumer.
- Duplicate events cannot double-apply installments or reduce exposure twice (partial unique index on succeeded attempts + transactional locking).
- Provider retry/settlement state is not mirrored in BNPL.
- Payload fingerprint conflicts are durably recorded without infinite Kafka retry loops.
- Contract completion requires zero remaining principal and no blocking customer debit obligations — not merely all installments marked paid.
- Checkout first-installment activation requires exact payable amount match (no silent overpayment).
