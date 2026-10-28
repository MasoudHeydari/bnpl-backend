# Aggregate cash collection and atomic installment settlement

BNPL owns principal, upfront, fees, commissions, tax, discounts and waivers. FC
receives one aggregate through existing generic protobufs and supplies authoritative
identity, amount, currency, result and effective settlement time. No BNPL fee type
or item list is added to an FC API.

A positive-net installment creates one complete immutable snapshot. Preparation
and checkout use that aggregate; FC success must exactly match it. Inside one
transaction, BNPL validates all current components, finalizes customer debits as
paid and consumed credits as waived, then marks the entire installment paid.
Only principal changes financed exposure; replay never releases it twice.

Paid is permanently closed. No residual-charge/charge-only repayment is supported.
Historical paid-with-outstanding-charge records are integrity findings, reported
read-only and requiring explicit reconciliation; no second payment or silent
historical rewrite is manufactured. Full rule, migration, locking and verification
contracts are in [database-invariants.md](../architecture/database-invariants.md).

Zero, negative and invalid economics have explicit safe lifecycle handling. New
incompatible issuance is blocked, existing obligations retain history/exposure,
automatic wrongful default/communication is suppressed, and precise reconciliation
blockers are exposed. The existing generic ReverseMerchantEntitlement RPC has
reversal economics; a safe funded noncash write-off/application contract preserving
merchant economics is not confirmed by the current BNPL integration. No unsupported
RPC or guessed financial success is introduced.
