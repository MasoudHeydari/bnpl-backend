package financialcorepaymentresultuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// WithCollectionsCoordination is wired once before consumers start. Payment
// application and Collections serialize on customer -> contract -> attempt ->
// installment -> charges -> credit account. No external call runs under locks.
func (uc *UseCase) WithCollectionsCoordination(profiles repo.CustomerProfile, reassess func(context.Context, model.CustomerID) error) *UseCase {
	uc.customerProfile = profiles
	uc.reassessCustomer = reassess
	return uc
}

// LockSettlementCustomerOnTx resolves immutable ownership without a row lock,
// then acquires customer and contract locks before any attempt/obligation lock.
func (uc *UseCase) LockSettlementCustomerOnTx(ctx context.Context, tx repo.Tx, id model.ContractID) error {
	if uc.customerProfile == nil || id.IsZero() {
		return nil
	}
	q := uc.contract.Tx(tx)
	contract, err := q.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if _, err = uc.customerProfile.Tx(tx).LockByID(ctx, contract.CustomerID); err != nil {
		return err
	}
	_, err = q.LockByID(ctx, id)
	return err
}

// ReassessSettledContract runs only after the monetary transaction committed.
// A failure is returned to the existing durable consumer for retry, including
// Inbox replays. Periodic assessment is the production-required safety net.
func (uc *UseCase) ReassessSettledContract(ctx context.Context, id model.ContractID) error {
	if uc.reassessCustomer == nil || id.IsZero() {
		return nil
	}
	var customer model.CustomerID
	err := uc.pool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		row, e := uc.contract.Conn(c).GetByID(ctx, id)
		customer = row.CustomerID
		return e
	})
	if err != nil {
		return err
	}
	return uc.reassessCustomer(ctx, customer)
}

// LockSettlementAttemptOnTx is the reconciliation entry point when the inbound
// envelope identifies an attempt rather than a contract.
func (uc *UseCase) LockSettlementAttemptOnTx(ctx context.Context, tx repo.Tx, id model.PaymentAttemptID) error {
	if uc.customerProfile == nil {
		return nil
	}
	attempt, err := uc.paymentAttempt.Tx(tx).GetByIDWithItems(ctx, id)
	if err != nil {
		return err
	}
	return uc.LockSettlementCustomerOnTx(ctx, tx, attempt.ContractID)
}
func (uc *UseCase) ReassessSettledAttempt(ctx context.Context, id model.PaymentAttemptID) error {
	if uc.reassessCustomer == nil {
		return nil
	}
	var contract model.ContractID
	err := uc.pool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		return c.ReadSnapshot(ctx, func(ctx context.Context, tx repo.Tx) error {
			row, e := uc.paymentAttempt.Tx(tx).GetByIDWithItems(ctx, id)
			contract = row.ContractID
			return e
		})
	})
	if err != nil {
		return err
	}
	return uc.ReassessSettledContract(ctx, contract)
}
