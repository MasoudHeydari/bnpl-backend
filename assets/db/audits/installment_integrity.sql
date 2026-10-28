-- Read-only preflight. Run before migration 000003 and during incident triage.
-- No business record is modified. Money is inventory arithmetic, not FC truth.
BEGIN TRANSACTION READ ONLY;
SELECT
    CASE WHEN i.status = 'paid' THEN
        'paid_with_unsettled_customer_component'
    ELSE
        'customer_component_settled_without_paid_installment'
    END AS issue,
    i.customer_id,
    i.contract_id,
    i.id AS installment_id,
    c.id AS charge_id,
    c.charge_type,
    c.charge_status,
    c.amount_minor,
    c.currency
FROM
    bnpl.installments i
    JOIN bnpl.installment_charges c ON c.installment_id = i.id
WHERE
    c.payer_type = 'customer'
    AND ((i.status = 'paid'
            AND c.charge_status IN ('offered', 'applied'))
        OR (i.status <> 'paid'
            AND (c.charge_status = 'paid'
                OR (c.charge_type IN ('discount', 'waiver')
                    AND c.charge_status = 'waived'))))
ORDER BY
    i.customer_id,
    i.contract_id,
    i.id,
    c.id;
SELECT
    'nonpositive_or_incompatible_customer_cash_obligation' AS issue,
    i.customer_id,
    i.contract_id,
    i.id AS installment_id,
    i.principal_amount_minor,
    i.upfront_amount_minor,
    i.currency,
    i.principal_amount_minor + i.upfront_amount_minor + coalesce(sum(
            CASE WHEN c.charge_type IN ('discount', 'waiver') THEN
                - c.amount_minor
            ELSE
                c.amount_minor
            END), 0) AS net_customer_cash_minor,
    array_agg(c.id) FILTER (WHERE c.id IS NOT NULL) AS charge_ids
FROM
    bnpl.installments i
    LEFT JOIN bnpl.installment_charges c ON c.installment_id = i.id
        AND c.payer_type = 'customer'
        AND c.charge_status IN ('offered', 'applied')
WHERE
    i.status IN ('offered', 'scheduled', 'due', 'overdue')
    AND i.principal_amount_minor > 0
GROUP BY
    i.id
HAVING
    i.principal_amount_minor + i.upfront_amount_minor + coalesce(sum(
            CASE WHEN c.charge_type IN ('discount', 'waiver') THEN
                - c.amount_minor
            ELSE
                c.amount_minor
            END), 0) <= 0
    OR bool_or(c.currency <> i.currency
        OR c.charge_type NOT IN ('bnpl_fee', 'commission', 'late_fee', 'tax', 'discount', 'waiver'))
ORDER BY
    i.customer_id,
    i.contract_id,
    i.id;
-- Historical charge-only / incomplete principal snapshots are audit findings,
-- not a supported route to collecting another payment.
SELECT
    'historical_incomplete_or_charge_only_snapshot' AS issue,
    i.customer_id,
    i.contract_id,
    i.id AS installment_id,
    p.id AS payment_attempt_id,
    p.state,
    p.financial_payment_intent_id,
    array_agg(x.source_id) FILTER (WHERE x.item_type = 'charge') AS charge_ids
FROM
    bnpl.payment_attempts p
    JOIN bnpl.installments i ON i.id = p.installment_id
    LEFT JOIN bnpl.payment_attempt_items x ON x.payment_attempt_id = p.id
GROUP BY
    i.id,
    p.id
HAVING
    count(*) FILTER (WHERE x.item_type = 'principal'
    AND x.source_id = i.id
    AND x.amount_minor = i.principal_amount_minor
    AND x.effect = 'debit') <> 1
ORDER BY
    i.customer_id,
    i.contract_id,
    i.id,
    p.id;
COMMIT;
