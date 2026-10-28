-- Post-migration 000003 inventory only. Do not classify old terminal-looking
-- records as authoritative without inspecting Financial Core and retained events.
BEGIN TRANSACTION READ ONLY;
SELECT i.customer_id,i.contract_id,i.id AS installment_id,p.id AS payment_attempt_id,
 p.state,p.failure_code,p.financial_payment_intent_id,p.created_at,p.updated_at
FROM bnpl.payment_attempts p JOIN bnpl.installments i ON i.id=p.installment_id
WHERE p.state IN ('failed','canceled') AND NOT p.financial_outcome_resolved
ORDER BY i.customer_id,i.contract_id,i.id,p.created_at,p.id;
COMMIT;
