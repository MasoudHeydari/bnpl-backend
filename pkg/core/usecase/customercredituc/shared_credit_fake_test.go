package customercredituc_test

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var fixtureCreditOwnerID = model.CreditOwnerID(uuid.MustParse("11111111-2222-4333-8444-555555555555"))

// noExistingSharedCredit implements SharedCredit for tests where the legal
// person has no prior grant.
type noExistingSharedCredit struct{}

func (noExistingSharedCredit) Tx(repo.Tx) repo.SharedCreditTxQueryer {
	return noExistingSharedCreditTx{}
}

type noExistingSharedCreditTx struct{}

func (noExistingSharedCreditTx) LockOwnerByCustomer(context.Context, model.CustomerID) (model.CreditOwnerID, error) {
	return fixtureCreditOwnerID, nil
}

func (noExistingSharedCreditTx) LockByOwnerAndKind(
	context.Context,
	model.CreditOwnerID,
	model.Currency,
	model.CreditKind,
) (model.CreditAccount, error) {
	return model.CreditAccount{}, cerr.ResourceNotFound(errors.New("no existing shared credit"))
}

func (noExistingSharedCreditTx) EnrollCustomer(
	context.Context,
	model.CustomerID,
	model.CreditAccountID,
	model.CreditOwnerID,
	time.Time,
) (bool, error) {
	return false, errors.New("unexpected enrollment: grant is not found")
}
