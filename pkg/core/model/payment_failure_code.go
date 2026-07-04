package model

// FinancialCorePaymentNotFoundFailureCode identifies authoritative Financial Core not_found results.
const FinancialCorePaymentNotFoundFailureCode = "financial_core_payment_not_found"

// PaymentSessionCreateRetryBudgetExhaustedFailureCode is retained for historical
// rows only. Sync CreateCheckoutPayment budget exhaustion must not invent an
// authoritative failed outcome; new paths use reconciliation_required instead.
const PaymentSessionCreateRetryBudgetExhaustedFailureCode = "payment_session_create_retry_budget_exhausted"

// PaymentSessionCreationAbandonedFailureCode is retained for historical rows
// only. Stale created attempts now enter reconciliation_required because
// created_timeout is not an authoritative Financial Core failure.
const PaymentSessionCreationAbandonedFailureCode = "payment_session_creation_abandoned"
