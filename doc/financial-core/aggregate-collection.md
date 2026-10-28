# Aggregate collection and local BNPL component accounting

Financial Core receives monetary operations through the existing protobufs. It
has no responsibility for BNPL fee, commission, tax, grace or discount rules.
No new FC protobuf or BNPL-aware FC operation is introduced or required here.

BNPL validates persisted customer components and creates an immutable
`payment_attempt_items` snapshot: principal and upfront cash plus customer-paid
fees, commission, late fees and tax, less discounts and waivers. Merchant-paid
charges do not increase the customer's collection. Arithmetic rejects overflow,
underflow, unsupported effects and invalid currency or component identities.

`PrepareCollectionObligation` and `CreateCheckoutPayment` receive the aggregate
amount and currency using the existing source/idempotency identities and
merchant-entitlement relationship. Preparation must return that same amount and
currency. No component-specific recognition is requested from FC.

The result consumer queries the existing authoritative `GetPaymentStatus` port.
After matching the financial identity, amount, currency and immutable local
snapshot, BNPL atomically applies the installment, all snapshotted customer
components (including consumed discounts/waivers), principal exposure release,
Inbox acknowledgement, audit and Outbox records. Current collectible charges
missing from the snapshot force reconciliation. Replays never release principal
again. Contract completion requires zero remaining principal, paid required
installments, no unpaid customer debit charges and no blocking payment attempt.

Historical principal-paid installments with residual customer debit charges
remain visible to Collections and repayment selection, including completed
contracts. A new payment snapshots only residual components, preserves the
original installment payment amount/time, and releases no additional principal.
The separate payment-attempt history records the additional collection.

A zero or negative net amount is not sent to a cash checkout. The existing
summary exposes `financial_adjustment_required` and the command rejects a
nonpositive collection. Such a balance requires an explicit economic adjustment;
this change never fabricates financial success, silently writes off debt, or
rewrites a historical snapshot. Aggregate FC operations must continue honoring
their existing idempotency and financial-accounting contract.
