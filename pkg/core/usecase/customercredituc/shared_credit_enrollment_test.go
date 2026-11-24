package customercredituc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type enrollmentCoordinator struct {
	account    model.CreditAccount
	accountErr error
	enrolled   int
	customerID model.CustomerID
}

func (c *enrollmentCoordinator) LockOwnerByCustomer(
	context.Context,
	repo.Tx,
	model.CustomerID,
) (model.CreditOwnerID, error) {
	return c.account.CreditOwnerID, nil
}

func (c *enrollmentCoordinator) LockByOwnerAndKind(
	_ context.Context,
	_ repo.Tx,
	ownerID model.CreditOwnerID,
	currency model.Currency,
	kind model.CreditKind,
) (model.CreditAccount, error) {
	if ownerID != c.account.CreditOwnerID || currency != c.account.Currency || kind != c.account.CreditKind {
		return model.CreditAccount{}, errors.New("unexpected shared credit lookup")
	}

	return c.account, c.accountErr
}

func (c *enrollmentCoordinator) EnrollCustomer(
	_ context.Context,
	_ repo.Tx,
	customerID model.CustomerID,
	_ model.CreditAccountID,
	_ model.CreditOwnerID,
	_ time.Time,
) (bool, error) {
	c.enrolled++
	c.customerID = customerID
	return true, nil
}

func TestSharedCreditEnrollmentReturnsExistingGrantForOtherAccount(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	ownerID := model.CreditOwnerID(uuid.New())
	originID := model.CustomerID(uuid.New())
	otherID := model.CustomerID(uuid.New())
	decisionID := model.DecisionID(uuid.New())

	account := model.CreditAccount{
		ID:                  model.CreditAccountID(uuid.New()),
		CustomerID:          originID,
		CreditOwnerID:       ownerID,
		Currency:            model.IRRCurrency,
		CreditKind:          model.PayInInstallmentsCreditKind,
		Status:              model.ActiveCreditAccountStatus,
		EffectiveFrom:       now.Add(-time.Hour),
		LastLimitDecisionID: &decisionID,
		LimitAmountMinor:    100000000,
	}
	coordinator := &enrollmentCoordinator{account: account}
	tx := mock.NewTx(t)
	decisionRepo := mock.NewDecision(t)
	decisionQ := mock.NewDecisionTxQueryer(t)
	decisionRepo.EXPECT().Tx(tx).Return(decisionQ).Once()
	decisionQ.EXPECT().GetByID(ctx, decisionID).Return(model.Decision{ID: decisionID}, nil).Once()
	uc := &UseCase{sharedCredit: coordinator, decision: decisionRepo}

	output, err := uc.enrollExistingSharedCredit(
		ctx,
		tx,
		model.CustomerProfile{ID: otherID, CreditOwnerID: ownerID},
		model.IRRCurrency,
		model.PayInInstallmentsCreditKind,
		now,
	)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, AssignInitialLimitResultAlreadyAssigned, output.Result)
	require.NotNil(t, output.CreditLimit)
	require.Equal(t, otherID, output.CreditLimit.CustomerID)
	require.Equal(t, originID, coordinator.account.CustomerID, "original grant must not change owner account")
	require.Equal(t, 1, coordinator.enrolled)
	require.Equal(t, otherID, coordinator.customerID)
}

func TestOtherAccountCannotRenewExpiredSharedCredit(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	ownerID := model.CreditOwnerID(uuid.New())
	originID := model.CustomerID(uuid.New())
	otherID := model.CustomerID(uuid.New())
	coordinator := &enrollmentCoordinator{account: model.CreditAccount{
		ID:            model.CreditAccountID(uuid.New()),
		CustomerID:    originID,
		CreditOwnerID: ownerID,
		Currency:      model.IRRCurrency,
		CreditKind:    model.PayInInstallmentsCreditKind,
		Status:        model.ExpiredCreditAccountStatus,
	}}
	uc := &UseCase{sharedCredit: coordinator}

	output, err := uc.enrollExistingSharedCredit(
		ctx,
		mock.NewTx(t),
		model.CustomerProfile{ID: otherID, CreditOwnerID: ownerID},
		model.IRRCurrency,
		model.PayInInstallmentsCreditKind,
		now,
	)
	require.Nil(t, output)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
	require.Zero(t, coordinator.enrolled)
}
