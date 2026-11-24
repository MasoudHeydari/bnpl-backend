package trustedmobilerp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.TrustedMobile = (*Repo)(nil)

// ErrNationalIDConflict means the mobile was previously imported with a
// different immutable national ID. Re-import must never overwrite it.
var ErrNationalIDConflict = repo.ErrTrustedMobileNationalIDConflict

type Repo struct{}

func New() *Repo { return &Repo{} }
func (*Repo) Conn(conn repo.Conn) repo.TrustedMobileQueryer {
	return connQueryer{postgres.MustConn(conn)}
}
func (*Repo) Tx(tx repo.Tx) repo.TrustedMobileTxQueryer { return txQueryer{postgres.MustTx(tx)} }

type connQueryer struct{ *postgres.Conn }
type txQueryer struct{ *postgres.Tx }

func (q connQueryer) GetByMobile(ctx context.Context, mobile string) (model.TrustedMobileNumber, error) {
	return getByMobile(ctx, q.Conn, mobile)
}
func (q txQueryer) GetByMobile(ctx context.Context, mobile string) (model.TrustedMobileNumber, error) {
	return getByMobile(ctx, q.Tx, mobile)
}
func getByMobile[Q postgres.Queryer](ctx context.Context, q Q, mobile string) (model.TrustedMobileNumber, error) {
	const query = `SELECT mobile_e164, national_id, created_by, created_at FROM trusted_mobile_numbers WHERE mobile_e164 = $1`
	return postgres.QueryOne[model.TrustedMobileNumber](ctx, q, "get trusted mobile", query, mobile)
}

func (q txQueryer) InsertIfAbsent(ctx context.Context, mobile, nationalID, actor string, now time.Time) (bool, error) {
	const op = "trustedmobilerp.InsertIfAbsent"
	if err := postgres.RequiredTime("created_at", now); err != nil {
		return false, err
	}
	const query = `INSERT INTO trusted_mobile_numbers (mobile_e164, national_id, created_by, created_at)
                   VALUES ($1, $2, $3, $4) ON CONFLICT (mobile_e164) DO NOTHING`
	rows, err := q.Tx.Exec(ctx, query, mobile, nationalID, actor, now)
	if err != nil {
		return false, fmt.Errorf("%s: insert: %w", op, err)
	}
	if rows == 1 {
		return true, nil
	}
	if rows != 0 {
		return false, fmt.Errorf("%s: unexpected affected row count %d", op, rows)
	}
	existing, err := q.GetByMobile(ctx, mobile)
	if err != nil {
		return false, fmt.Errorf("%s: check existing pair: %w", op, err)
	}
	if existing.NationalID != nationalID {
		return false, ErrNationalIDConflict
	}
	return false, nil
}
