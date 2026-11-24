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

// noExistingSharedCredit implements the mandatory person-level lock/lookup
// operations while emulating a legal person with no prior grant.
type noExistingSharedCredit struct{}

func (noExistingSharedCredit) LockOwnerByCustomer(context.Context, repo.Tx, model.CustomerID) (model.CreditOwnerID, error) {
	return fixtureCreditOwnerID, nil
}

func (noExistingSharedCredit) LockByOwnerAndKind(
	context.Context,
	repo.Tx,
	model.CreditOwnerID,
	model.Currency,
	model.CreditKind,
) (model.CreditAccount, error) {
	return model.CreditAccount{}, cerr.ResourceNotFound(errors.New("no existing shared credit"))
}

func (noExistingSharedCredit) EnrollCustomer(
	context.Context,
	repo.Tx,
	model.CustomerID,
	model.CreditAccountID,
	model.CreditOwnerID,
	time.Time,
) (bool, error) {
	return false, errors.New("unexpected enrollment: grant is not found")
}
