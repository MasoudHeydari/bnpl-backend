package checkoutsessionuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// harnessCreditAccountRepo satisfies repo.CreditAccount and returns a
// product-aware Conn queryer for Partner Create credit-kind previews.
type harnessCreditAccountRepo struct {
	conn *mock.CreditAccountConnQueryer
}

func (r *harnessCreditAccountRepo) Conn(repo.Conn) repo.CreditAccountConnQueryer {
	return &harnessCreditAccountConn{CreditAccountConnQueryer: r.conn}
}

func (r *harnessCreditAccountRepo) Tx(repo.Tx) repo.CreditAccountTxQueryer {
	panic("harnessCreditAccountRepo.Tx is unused in checkout-session unit tests")
}

type harnessCreditAccountConn struct {
	repo.CreditAccountConnQueryer
}

func (c *harnessCreditAccountConn) GetByCustomerIDCurrencyAndKind(
	context.Context, model.CustomerID, model.Currency, model.CreditKind,
) (model.CreditAccount, error) {
	return model.CreditAccount{}, cerr.ResourceNotFound(nil)
}

func (c *harnessCreditAccountConn) ListByCustomerID(
	context.Context, model.CustomerID,
) ([]model.CreditAccount, error) {
	return nil, nil
}

func (c *harnessCreditAccountConn) HasUnpaidMonthlyCycleBefore(
	context.Context, model.CustomerID, model.Currency, time.Time,
) (bool, error) {
	return false, nil
}
