package creditallocationrp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.CreditAllocation = (*Repo)(nil)

type Repo struct{}

func New() *Repo                                           { return &Repo{} }
func (*Repo) Tx(tx repo.Tx) repo.CreditAllocationTxQueryer { return txQueryer{postgres.MustTx(tx)} }

type txQueryer struct{ *postgres.Tx }

// The singleton row is a configuration fence, not a lock on credit balances.
func (q txQueryer) LockConfiguration(ctx context.Context, write bool) error {
	if write {
		n, err := q.Exec(ctx, `UPDATE credit_allocation_control SET revision=revision+1 WHERE id=1`)
		if err != nil {
			return postgres.MapError(err)
		}
		if n != 1 {
			return fmt.Errorf("credit allocation control is missing")
		}
		return nil
	}
	type row struct {
		Revision int64 `db:"revision"`
	}
	_, err := postgres.QueryOne[row](ctx, q.Tx, "lock credit allocation configuration", `SELECT revision FROM credit_allocation_control WHERE id=1 FOR SHARE`)
	return err
}

func (q txQueryer) GetGroup(ctx context.Context, code string) (model.CreditGroup, error) {
	return postgres.QueryOne[model.CreditGroup](ctx, q.Tx, "get credit group", `SELECT * FROM credit_groups WHERE code=$1`, code)
}
func (q txQueryer) ListGroups(ctx context.Context) ([]model.CreditGroup, error) {
	return postgres.QueryAll[model.CreditGroup](ctx, q.Tx, "list credit groups", `SELECT * FROM credit_groups ORDER BY code`)
}
func (q txQueryer) PutGroup(ctx context.Context, g model.CreditGroup) (model.CreditGroup, error) {
	return postgres.QueryOne[model.CreditGroup](ctx, q.Tx, "put credit group", `INSERT INTO credit_groups(code,name,enabled,updated_by,updated_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(code) DO UPDATE SET name=excluded.name,enabled=excluded.enabled,updated_by=excluded.updated_by,updated_at=excluded.updated_at RETURNING *`, g.Code, g.Name, g.Enabled, g.UpdatedBy, g.UpdatedAt)
}
func (q txQueryer) GetEligibilityByNationalCode(ctx context.Context, code string) (model.CreditEligibilityEntry, error) {
	return postgres.QueryOne[model.CreditEligibilityEntry](ctx, q.Tx, "get credit eligibility by national code", `SELECT * FROM credit_eligibility_entries WHERE national_code=$1`, code)
}
func (q txQueryer) GetEligibilityByMobile(ctx context.Context, mobile string) (model.CreditEligibilityEntry, error) {
	return postgres.QueryOne[model.CreditEligibilityEntry](ctx, q.Tx, "get credit eligibility by mobile", `SELECT e.* FROM credit_eligibility_entries e JOIN credit_eligibility_mobiles m ON m.eligibility_id=e.id WHERE m.mobile_e164=$1`, mobile)
}
func (q txQueryer) GetEligibility(ctx context.Context, id uuid.UUID) (model.CreditEligibilityEntry, error) {
	return postgres.QueryOne[model.CreditEligibilityEntry](ctx, q.Tx, "get credit eligibility", `SELECT * FROM credit_eligibility_entries WHERE id=$1`, id)
}
func (q txQueryer) InsertEligibility(ctx context.Context, e model.CreditEligibilityEntry) (model.CreditEligibilityEntry, error) {
	return postgres.QueryOne[model.CreditEligibilityEntry](ctx, q.Tx, "insert credit eligibility", `INSERT INTO credit_eligibility_entries(national_code,created_by,created_at) VALUES($1,$2,$3) RETURNING *`, e.NationalCode, e.CreatedBy, e.CreatedAt)
}
func (q txQueryer) BindEligibility(ctx context.Context, id uuid.UUID, owner model.CreditOwnerID) error {
	n, err := q.Exec(ctx, `UPDATE credit_eligibility_entries e SET credit_owner_id=$2 FROM credit_owners o WHERE e.id=$1 AND o.id=$2 AND o.national_code=e.national_code AND (e.credit_owner_id IS NULL OR e.credit_owner_id=$2)`, id, owner)
	if err != nil {
		return postgres.MapError(err)
	}
	if n != 1 {
		return cerr.Conflict(fmt.Errorf("eligibility owner identity mismatch"))
	}
	return nil
}
func (q txQueryer) GetMembership(ctx context.Context, id uuid.UUID) (model.CreditGroupMembership, error) {
	return postgres.QueryOne[model.CreditGroupMembership](ctx, q.Tx, "get credit group membership", `SELECT * FROM credit_group_memberships WHERE eligibility_id=$1 ORDER BY version DESC LIMIT 1`, id)
}
func (q txQueryer) PutMembership(ctx context.Context, m model.CreditGroupMembership) (model.CreditGroupMembership, error) {
	return postgres.QueryOne[model.CreditGroupMembership](ctx, q.Tx, "put credit group membership", `INSERT INTO credit_group_memberships(eligibility_id,version,group_code,created_by,created_at) VALUES($1,$2,$3,$4,$5) RETURNING *`, m.EligibilityID, m.Version, m.GroupCode, m.CreatedBy, m.CreatedAt)
}
func (q txQueryer) GetRule(ctx context.Context, scope model.CreditAllocationScope, group *string, entry *uuid.UUID, kind model.CreditKind, currency model.Currency) (model.CreditAllocationRule, error) {
	return postgres.QueryOne[model.CreditAllocationRule](ctx, q.Tx, "get credit allocation rule", `SELECT * FROM credit_allocation_rules WHERE scope=$1 AND group_code IS NOT DISTINCT FROM $2::text AND eligibility_id IS NOT DISTINCT FROM $3::uuid AND credit_kind=$4 AND currency=$5 ORDER BY version DESC LIMIT 1`, scope, group, entry, kind, currency)
}
func (q txQueryer) PutRule(ctx context.Context, r model.CreditAllocationRule) (model.CreditAllocationRule, error) {
	return postgres.QueryOne[model.CreditAllocationRule](ctx, q.Tx, "put credit allocation rule", `INSERT INTO credit_allocation_rules(scope,group_code,eligibility_id,credit_kind,currency,strategy,fixed_amount_minor,version,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *`, r.Scope, r.GroupCode, r.EligibilityID, r.CreditKind, r.Currency, r.Strategy, r.FixedAmountMinor, r.Version, r.CreatedBy, r.CreatedAt)
}
func (q txQueryer) ListRules(ctx context.Context) ([]model.CreditAllocationRule, error) {
	return postgres.QueryAll[model.CreditAllocationRule](ctx, q.Tx, "list current credit allocation rules", `SELECT DISTINCT ON (scope,group_code,eligibility_id,credit_kind,currency) * FROM credit_allocation_rules ORDER BY scope,group_code,eligibility_id,credit_kind,currency,version DESC`)
}
func (q txQueryer) GetOpenRequest(ctx context.Context, owner model.CreditOwnerID, kind model.CreditKind, currency model.Currency) (model.CreditRequest, error) {
	return postgres.QueryOne[model.CreditRequest](ctx, q.Tx, "get open credit request", `SELECT * FROM credit_requests WHERE credit_owner_id=$1 AND credit_kind=$2 AND currency=$3 AND status IN ('pending','processing')`, owner, kind, currency)
}
func (q txQueryer) GetLatestRequest(ctx context.Context, customer model.CustomerID, kind model.CreditKind, currency model.Currency) (model.CreditRequest, error) {
	return postgres.QueryOne[model.CreditRequest](ctx, q.Tx, "get latest customer credit request", `SELECT * FROM credit_requests WHERE customer_id=$1 AND credit_kind=$2 AND currency=$3 ORDER BY requested_at DESC,id DESC LIMIT 1`, customer, kind, currency)
}
func (q txQueryer) GetRequest(ctx context.Context, id uuid.UUID, lock bool) (model.CreditRequest, error) {
	query := `SELECT * FROM credit_requests WHERE id=$1`
	if lock {
		query += ` FOR UPDATE`
	}
	return postgres.QueryOne[model.CreditRequest](ctx, q.Tx, "get credit request", query, id)
}
func (q txQueryer) InsertRequest(ctx context.Context, r model.CreditRequest) (model.CreditRequest, error) {
	return postgres.QueryOne[model.CreditRequest](ctx, q.Tx, "insert credit request", `INSERT INTO credit_requests(credit_owner_id,customer_id,external_customer_id,mobile_e164,national_code,credit_kind,currency,status,queue_source,next_attempt_at,requested_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8,$9,$10,$10) RETURNING *`, r.CreditOwnerID, r.CustomerID, r.ExternalCustomerID, r.MobileE164, r.NationalCode, r.CreditKind, r.Currency, r.QueueSource, r.NextAttemptAt, r.RequestedAt)
}
func (q txQueryer) ClaimRequest(ctx context.Context, id uuid.UUID, now, until time.Time) (model.CreditRequest, error) {
	return postgres.QueryOne[model.CreditRequest](ctx, q.Tx, "claim credit request", `UPDATE credit_requests SET status='processing',lease_token=gen_random_uuid(),lease_until=$3,updated_at=$2 WHERE id=$1 AND queue_source IS NOT NULL AND next_attempt_at<=$2 AND (status='pending' OR (status='processing' AND lease_until<=$2)) RETURNING *`, id, now, until)
}

// QueuePublicRequests wakes a bounded batch of dormant requests after public
// risk assessment becomes available. Current rules are rechecked before grant.
// A disabled named rule must not repeatedly wake through the public default.
func (q txQueryer) QueuePublicRequests(ctx context.Context, now time.Time, limit int) error {
	_, err := q.Exec(ctx, `WITH selected AS (
 SELECT r.id FROM credit_requests r
 LEFT JOIN credit_eligibility_entries e ON e.national_code=r.national_code AND (e.credit_owner_id=r.credit_owner_id OR EXISTS (SELECT 1 FROM credit_eligibility_mobiles em WHERE em.eligibility_id=e.id AND em.mobile_e164=r.mobile_e164))
 LEFT JOIN LATERAL (SELECT group_code FROM credit_group_memberships WHERE eligibility_id=e.id ORDER BY version DESC LIMIT 1) m ON true
 LEFT JOIN credit_groups g ON g.code=m.group_code
 LEFT JOIN LATERAL (SELECT strategy FROM credit_allocation_rules WHERE scope='individual' AND eligibility_id=e.id AND credit_kind=r.credit_kind AND currency=r.currency ORDER BY version DESC LIMIT 1) i ON true
 LEFT JOIN LATERAL (SELECT strategy FROM credit_allocation_rules WHERE scope='group' AND group_code=m.group_code AND credit_kind=r.credit_kind AND currency=r.currency ORDER BY version DESC LIMIT 1) gr ON true
 JOIN LATERAL (SELECT id,strategy FROM credit_allocation_rules WHERE scope='default' AND credit_kind=r.credit_kind AND currency=r.currency ORDER BY version DESC LIMIT 1) d ON true
 WHERE r.status='pending' AND r.queue_source IS NULL AND d.strategy='risk_based'
 AND (i.strategy IS NULL OR i.strategy='inherit')
 AND (m.group_code IS NULL OR (g.enabled AND (gr.strategy IS NULL OR gr.strategy='inherit')))
 AND r.allocation_rule_id IS DISTINCT FROM d.id
 ORDER BY r.requested_at,r.id LIMIT $2 FOR UPDATE OF r SKIP LOCKED
 ) UPDATE credit_requests r SET queue_source='public',next_attempt_at=$1,updated_at=$1 FROM selected s WHERE r.id=s.id`, now, limit)
	return postgres.MapError(err)
}

func (q txQueryer) ListDueRequests(ctx context.Context, now time.Time, limit int) ([]model.CreditRequest, error) {
	return postgres.QueryAll[model.CreditRequest](ctx, q.Tx, "list due credit requests", `SELECT * FROM credit_requests WHERE queue_source IS NOT NULL AND next_attempt_at<=$1 AND (status='pending' OR (status='processing' AND lease_until<=$1)) ORDER BY next_attempt_at,id LIMIT $2`, now, limit)
}
func (q txQueryer) FinishRequest(ctx context.Context, r model.CreditRequest, status model.CreditRequestStatus, reason string, decision *model.DecisionID, rule *uuid.UUID, next *time.Time, now time.Time) (model.CreditRequest, error) {
	return postgres.QueryOne[model.CreditRequest](ctx, q.Tx, "finish credit request", `UPDATE credit_requests SET status=$3,reason_code=$4,decision_id=$5,allocation_rule_id=$6,next_attempt_at=$7,queue_source=CASE WHEN $7::timestamptz IS NULL THEN NULL ELSE queue_source END,lease_token=NULL,lease_until=NULL,updated_at=$8,completed_at=CASE WHEN $3 IN ('approved','rejected','canceled') THEN $8 ELSE NULL END WHERE id=$1 AND lease_token=$2 AND status='processing' AND lease_until>$8 RETURNING *`, r.ID, r.LeaseToken, status, reason, decision, rule, next, now)
}

// QueueEligibility authorizes only matching named persons/groups. An already
// queued request is not replaced, and active leases are never stolen.
func (q txQueryer) QueueEligibility(ctx context.Context, entry *uuid.UUID, group *string, kind *model.CreditKind, currency *model.Currency, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE credit_requests r SET queue_source='eligibility',next_attempt_at=$4,updated_at=$4 FROM credit_eligibility_entries e WHERE r.status='pending' AND r.queue_source IS NULL AND r.national_code=e.national_code AND ($1::uuid IS NULL OR e.id=$1) AND ($3::text IS NULL OR r.credit_kind=$3) AND ($5::text IS NULL OR r.currency=$5) AND ($2::text IS NULL OR $2=(SELECT m.group_code FROM credit_group_memberships m WHERE m.eligibility_id=e.id ORDER BY m.version DESC LIMIT 1))`, entry, group, kind, now, currency)
	return postgres.MapError(err)
}
func (q txQueryer) ListPendingRequests(ctx context.Context, kind model.CreditKind, before time.Time, limit int) ([]model.CreditRequest, error) {
	return postgres.QueryAll[model.CreditRequest](ctx, q.Tx, "list pending credit requests", `SELECT * FROM credit_requests WHERE credit_kind=$1 AND status IN ('pending','processing') AND requested_at<=$2 ORDER BY requested_at DESC,id DESC LIMIT $3`, kind, before, limit)
}

func (q txQueryer) InsertEligibilityMobile(ctx context.Context, m model.CreditEligibilityMobile) error {
	_, err := q.Exec(ctx, `INSERT INTO credit_eligibility_mobiles(mobile_e164,eligibility_id,created_by,created_at) VALUES($1,$2,$3,$4)`, m.MobileE164, m.EligibilityID, m.CreatedBy, m.CreatedAt)
	return postgres.MapError(err)
}
func (q txQueryer) ListEligibility(ctx context.Context, group *string, after *uuid.UUID, limit int) ([]model.CreditEligibilityEntry, error) {
	return postgres.QueryAll[model.CreditEligibilityEntry](ctx, q.Tx, "list eligibility entries", `SELECT e.* FROM credit_eligibility_entries e WHERE ($1::text IS NULL OR $1=(SELECT m.group_code FROM credit_group_memberships m WHERE m.eligibility_id=e.id ORDER BY m.version DESC LIMIT 1)) AND ($2::uuid IS NULL OR e.id>$2) ORDER BY e.id LIMIT $3`, group, after, limit)
}
func (q txQueryer) ListEligibilityMobiles(ctx context.Context, id uuid.UUID) ([]model.CreditEligibilityMobile, error) {
	return postgres.QueryAll[model.CreditEligibilityMobile](ctx, q.Tx, "list eligibility mobiles", `SELECT * FROM credit_eligibility_mobiles WHERE eligibility_id=$1 ORDER BY mobile_e164`, id)
}
func (q txQueryer) QueueIdentityRecheck(ctx context.Context, id uuid.UUID, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE credit_requests SET queue_source='identity_recheck',next_attempt_at=$2,updated_at=$2 WHERE id=$1 AND status='pending' AND queue_source IS NULL`, id, now)
	return postgres.MapError(err)
}
