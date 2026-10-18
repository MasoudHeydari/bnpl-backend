package customercredituc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	initialcreditlimitv1 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

func newSummaryUseCase(
	t *testing.T,
	pool *mock.Pool,
	customerProfile *mock.CustomerProfile,
	creditAccount *mock.CreditAccount,
	policyAssignment *mock.PolicyAssignment,
	clk *mock.Clock,
) *customercredituc.UseCase {
	t.Helper()

	policy := initialcreditlimitv1.New()
	registry, err := initialcreditlimit.NewRegistry(policy)
	require.NoError(t, err)

	uc, err := customercredituc.New(
		pool,
		customerProfile,
		creditAccount,
		mock.NewDecision(t),
		mock.NewOutbox(t),
		clk,
		sha256fingerprint.New(),
		mock.NewCreditAssessmentClient(t),
		policyAssignment,
		registry,
		policy.Ref(),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
	)
	require.NoError(t, err)

	return uc
}

func expectSummaryPolicyAssignmentDefault(
	t *testing.T,
	tx *mock.Tx,
	policyAssignment *mock.PolicyAssignment,
) {
	t.Helper()

	txQ := mock.NewPolicyAssignmentTxQueryer(t)
	policyAssignment.EXPECT().Tx(tx).Return(txQ).Once()
	txQ.EXPECT().GetActive(
		testifymock.Anything,
		model.CustomerPolicySubjectType,
		testifymock.Anything,
		model.InitialCreditLimitPolicyFamily,
		testifymock.Anything,
	).Return(model.PolicyAssignment{}, cerr.ResourceNotFound(errors.New("missing"))).Once()
}

func TestGetSummary_ExpiredLimit_ReturnsExpiredNotZeroAssigned(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	clk := mock.NewClock(t)

	customerProfile := mock.NewCustomerProfile(t)
	customerProfileTx := mock.NewCustomerProfileTxQueryer(t)
	creditAccount := mock.NewCreditAccount(t)
	creditAccountTx := mock.NewCreditAccountTxQueryer(t)
	policyAssignment := mock.NewPolicyAssignment(t)

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	profileID := model.CustomerID(uuid.New())
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Hour)
	effectiveFrom := now.Add(-30 * 24 * time.Hour)
	accountID := model.CreditAccountID(uuid.New())

	clk.EXPECT().Now().Return(now)

	pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, conn)
		})
	conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, tx)
		}).Once()

	customerProfile.EXPECT().Tx(tx).Return(customerProfileTx).Once()
	customerProfileTx.EXPECT().
		EnsureExists(
			testifymock.Anything,
			externalCustomerID,
			model.IRJurisdiction,
			model.ActiveCustomerStatus,
			now,
			now,
		).
		Return(model.CustomerProfile{
			ID:                profileID,
			ExternalCustomerID: externalCustomerID,
			UpdatedAt:         now,
		}, true, nil).
		Once()

	creditAccount.EXPECT().Tx(tx).Return(creditAccountTx).Once()
	expectSummaryPolicyAssignmentDefault(t, tx, policyAssignment)
	creditAccountTx.EXPECT().
		ExpireDueByCustomerIDAndCurrency(
			testifymock.Anything,
			profileID,
			model.IRRCurrency,
			now,
		).
		Return(nil).
		Once()

	expiredAccount := model.CreditAccount{
		ID:               accountID,
		CustomerID:       profileID,
		LimitAmountMinor: 5_000_000,
		Currency:         model.IRRCurrency,
		Status:           model.ExpiredCreditAccountStatus,
		EffectiveFrom:    effectiveFrom,
		ExpiresAt:        &expiredAt,
		UpdatedAt:        now,
	}

	creditAccountTx.EXPECT().
		GetByCustomerIDAndCurrency(testifymock.Anything, profileID, model.IRRCurrency).
		Return(expiredAccount, nil).
		Once()

	uc := newSummaryUseCase(t, pool, customerProfile, creditAccount, policyAssignment, clk)

	out, err := uc.GetSummary(context.Background(), customercredituc.Input{
		ExternalCustomerID: externalCustomerID,
	})
	require.NoError(t, err)
	require.Equal(t, customercredituc.SummaryStatusExpired, out.Status)
	require.Equal(t, uint64(0), out.AvailableAmountMinor)
	require.NotNil(t, out.LimitStatus)
	require.Equal(t, model.ExpiredCreditAccountStatus, *out.LimitStatus)
	require.NotEqual(t, customercredituc.SummaryStatusNotAssigned, out.Status)
}

func TestGetSummary_NoLimit_ReturnsNotAssigned(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	clk := mock.NewClock(t)

	customerProfile := mock.NewCustomerProfile(t)
	customerProfileTx := mock.NewCustomerProfileTxQueryer(t)
	creditAccount := mock.NewCreditAccount(t)
	creditAccountTx := mock.NewCreditAccountTxQueryer(t)
	policyAssignment := mock.NewPolicyAssignment(t)

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	profileID := model.CustomerID(uuid.New())
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

	clk.EXPECT().Now().Return(now)
	pool.EXPECT().
		Conn(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
			return fn(ctx, conn)
		})
	conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, tx)
		}).Once()
	customerProfile.EXPECT().Tx(tx).Return(customerProfileTx).Once()
	customerProfileTx.EXPECT().
		EnsureExists(
			testifymock.Anything,
			externalCustomerID,
			model.IRJurisdiction,
			model.ActiveCustomerStatus,
			now,
			now,
		).
		Return(model.CustomerProfile{
			ID:                profileID,
			ExternalCustomerID: externalCustomerID,
			UpdatedAt:         now,
		}, true, nil).
		Once()

	creditAccount.EXPECT().Tx(tx).Return(creditAccountTx).Once()
	expectSummaryPolicyAssignmentDefault(t, tx, policyAssignment)
	creditAccountTx.EXPECT().
		ExpireDueByCustomerIDAndCurrency(testifymock.Anything, profileID, model.IRRCurrency, now).
		Return(nil).
		Once()
	creditAccountTx.EXPECT().
		GetByCustomerIDAndCurrency(testifymock.Anything, profileID, model.IRRCurrency).
		Return(model.CreditAccount{}, cerr.ResourceNotFound(errors.New("none"))).
		Once()

	uc := newSummaryUseCase(t, pool, customerProfile, creditAccount, policyAssignment, clk)

	out, err := uc.GetSummary(context.Background(), customercredituc.Input{
		ExternalCustomerID: externalCustomerID,
	})
	require.NoError(t, err)
	require.Equal(t, customercredituc.SummaryStatusNotAssigned, out.Status)
	require.Equal(t, uint64(0), out.AvailableAmountMinor)
	require.Nil(t, out.LimitStatus)
}
