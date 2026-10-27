# BNPL correctness continuation verification

Starting point: `a06d245498b8dfc1c2d67114894512a465bd2c68` on
`feat-add-decision-domain`. No PR or schema migration is part of this change.

## Existing work preserved

The preceding 34-file change already introduced hashed Collections Decision
fingerprints, opening/recovery lineage, read-only credit guards with live
eligibility checks, episode-scoped restriction recovery checks, uncertainty
event comparison, batch failure reporting, and ShortLink configuration cleanup.
The removed repayment endpoints, enable flag, and static alias remain removed.
Existing accepted-operation replay and Financial Core settlement paths are
preserved.

## Completed changes

- Record uncertainty resolution as an empty state in the existing uncertainty
  event stream. Unchanged states create no new events; the same installment set
  can recur after resolution. Microsecond ordering handles repeated clock values.
- Bound batch error retention to ten samples while logging every failed customer
  with structured identifiers. Preserve partial counts when candidate listing
  subsequently fails. Existing bounded outcome metric labels remain unchanged.
- Share obligation-to-policy mapping and the installment grace boundary between
  Collections and live credit summary checks.
- Honor persisted grace snapshots without a 31-day clamp. UTC date arithmetic
  preserves elapsed 24-hour days without duration overflow. PostgreSQL overdue
  selection uses `24 hours`, avoiding session-timezone DST differences. Candidate
  scanning intentionally includes obligations inside grace for recovery checks.
- Keep reminder calendar dates and delivery windows correct across DST; suppress
  grace messages at/after actual contractual expiry and identify the final
  calendar window before that instant. Include longer persisted grace snapshots
  beyond the normal reminder lookback.
- Read paginated unpaid installments and the global repayment summary inside one
  read-only repeatable-read snapshot. Preserve global priority and the documented
  scope of `has_pending_payment` (the selected priority target).
- Do not advertise a payable amount when current Financial Core recognition
  permits only principal and the command would reject charges.
- Reject malformed public short-link hosts before dispatch, including schemes,
  paths, credentials, and ports. Disabled dispatch accepts absent configuration.
- Regenerate affected repository mocks. Correct the stale expected task count
  from twelve to thirteen.

## Verification

Executed successfully:

- `go test -tags development ./...`
- `go test -race -tags 'development integration' ./pkg/core/usecase/collectionsuc -run TestPostgreSQL -v`
  with `COLLECTIONS_TEST_DATABASE_URL` pointing to a dedicated PostgreSQL 18
  container, separate from the existing application database.
- `go test -tags 'development integration' ./pkg/adapter/db/postgres/decisionrp -run TestCollectionsDecisionFingerprint -v`
  with the dedicated test database's `POSTGRES_*` environment.
- `go test -tags production ./cmd/bnpl/... -run '^$'`
- `git diff --check` and obsolete ShortLink flag/alias reference search.

The PostgreSQL suite exercises concurrent opening, immutable Decision lineage,
recovery with future obligations remaining, independent subsequent episodes,
restriction mismatch refusal, opening/recovery rollback, historical retention,
uncertainty transition idempotency, repeated timestamps, coherent snapshots during
settlement, bounded batch errors, SQL grace boundaries across both DST changes,
zero and 45-day grace, delayed lifecycle jobs, and longer-grace reminder scanning.
It also verifies Reminder + ShortLink + Outbox rollback on an injected outbox
failure, successful retry, repeated dispatch deduplication, persisted-link HTTP
redirects, and coexistence with API, health and Swagger routes.

Both Collections integration tests now fail explicitly when their required
PostgreSQL configuration/connection is unavailable; no database verification is
reported from skipped tests. The entire external-service E2E suite was not run.

## Domain and data safeguards

The `decisions` table, baseline DDL, incremental migrations and general Decision
repository implementation are unchanged. No physical deletion of episodes,
restrictions, cases, events or short links was introduced. Recovery releases only
the active restriction linked to the episode. Login, reading and repayment remain
outside the new-credit guard. Authoritative settlement still belongs to the
Financial Core result consumer; no external calls were added inside transactions.
No public endpoint was added or restored, and Swagger regeneration was unnecessary.

## Deployment limitation

The currently mounted local Envoy configuration belongs to the separate
`platform-infra` checkout. It preserves request authority but has no short-link
host route for `/<code>`; its root fallback returns 404. Backend routing and real
persisted-link redirects pass HTTP integration tests, but a public redirect through
that deployment cannot work until its generated routing configuration adds a
host-scoped route to `bnpl_backend` before the fallback, preserves the Host, and
allows unauthenticated redirect requests. This change does not modify that
separate infrastructure repository or restart shared services. Live public-domain
DNS/TLS and live Envoy redirect delivery were not verified.

## Files changed

- `cmd/bnpl/bootstrap/usecase.go`
- `doc/collections-correctness-verification.md`
- `pkg/adapter/config/infrastructure_test.go`
- `pkg/adapter/config/repayment_shortlink_test.go`
- `pkg/adapter/config/validate.go`
- `pkg/adapter/db/postgres/conn.go`
- `pkg/adapter/db/postgres/customerprofilerp/repo.go`
- `pkg/adapter/db/postgres/decisionrp/collections_fingerprint_integration_test.go`
- `pkg/adapter/db/postgres/installmentrp/query.go`
- `pkg/core/mock/mock_repo_Conn.go`
- `pkg/core/mock/mock_repo_CustomerProfileTxQueryer.go`
- `pkg/core/model/installment.go`
- `pkg/core/policy/collections/v1/policy.go`
- `pkg/core/policy/collections/v1/policy_test.go`
- `pkg/core/policy/repaymentreminder/v1/policy.go`
- `pkg/core/policy/repaymentreminder/v1/policy_test.go`
- `pkg/core/policy/shortlinkdestination/policy.go`
- `pkg/core/repo/conn.go`
- `pkg/core/repo/customer_profilerp.go`
- `pkg/core/usecase/collectionsuc/assess.go`
- `pkg/core/usecase/collectionsuc/lifecycle_integration_test.go`
- `pkg/core/usecase/collectionsuc/usecase.go`
- `pkg/core/usecase/customeraccountuc/list_unpaid_installments.go`
- `pkg/core/usecase/customeraccountuc/list_unpaid_installments_test.go`
- `pkg/core/usecase/customeraccountuc/repayment_summary_test.go`
- `pkg/core/usecase/customercredituc/collections_guard.go`
- `pkg/core/usecase/repaymentreminderuc/usecase.go`
