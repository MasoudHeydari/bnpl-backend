package customercredituc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	collectionsv2 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v2"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcustomer "github.com/parstis/bnpl-backend/pkg/core/external/customer"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit"
	initialcreditlimitv1 "github.com/parstis/bnpl-backend/pkg/core/policy/initialcreditlimit/v1"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

const summaryTestMobileE164 = "+989121234567"

func newSummaryUseCase(
	t *testing.T,
	pool *mock.Pool,
	customerProfile *mock.CustomerProfile,
	creditAccount *mock.CreditAccount,
	policyAssignment *mock.PolicyAssignment,
	clk *mock.Clock,
	decision *mock.Decision,
	customerRestriction *mock.CustomerRestriction,
	installment *mock.Installment,
	defaultEpisode *mock.DefaultEpisode,
	customerResolver *mock.CustomerResolver,
) *customercredituc.UseCase {
	t.Helper()

	policy := initialcreditlimitv1.New()
	registry, err := initialcreditlimit.NewRegistry(policy)
	require.NoError(t, err)

	if decision == nil {
		decision = mock.NewDecision(t)
	}

	if customerRestriction == nil {
		customerRestriction = mock.NewCustomerRestriction(t)
	}

	if installment == nil {
		installment = mock.NewInstallment(t)
	}

	if defaultEpisode == nil {
		defaultEpisode = mock.NewDefaultEpisode(t)
	}

	if customerResolver == nil {
		customerResolver = mock.NewCustomerResolver(t)
	}

	uc, err := customercredituc.New(
		pool,
		customerProfile,
		creditAccount,
		decision,
		customerRestriction,
		installment,
		defaultEpisode,
		mock.NewOutbox(t),
		clk,
		sha256fingerprint.New(),
		mock.NewCreditAssessmentClient(t),
		customerResolver,
		policyAssignment,
		registry,
		policy.Ref(),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
		collectionsv2.New(time.UTC),
	)
	require.NoError(t, err)

	return uc
}

func expectSummaryResolveActive(
	t *testing.T,
	resolver *mock.CustomerResolver,
	externalCustomerID model.ExternalCustomerID,
) {
	t.Helper()

	resolver.EXPECT().ResolveByMobile(
		testifymock.Anything,
		extcustomer.ResolveByMobileInput{MobileE164: summaryTestMobileE164},
	).Return(extcustomer.ResolveByMobileOutput{
		ExternalCustomerID: externalCustomerID,
		Status:             extcustomer.ActiveStatus,
	}, nil).Once()
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
	now := time.Date(
		2026,
		8,
		9,
		12,
		0,
		0,
		0,
		time.UTC,
	)
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
			testifymock.Anything,
			externalCustomerID,
			model.IRJurisdiction,
			model.ActiveCustomerStatus,
			now,
			now,
		).
		Return(model.CustomerProfile{
			ID:                 profileID,
			ExternalCustomerID: externalCustomerID,
			UpdatedAt:          now,
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

	customerRestriction := mock.NewCustomerRestriction(t)
	customerRestrictionTx := mock.NewCustomerRestrictionTxQueryer(t)
	customerRestriction.EXPECT().Tx(tx).Return(customerRestrictionTx).Once()
	customerRestrictionTx.EXPECT().
		ListActiveByCustomerID(testifymock.Anything, profileID).
		Return(nil, nil).
		Once()

	defaultEpisode := mock.NewDefaultEpisode(t)
	defaultEpisodeTx := mock.NewDefaultEpisodeTxQueryer(t)
	defaultEpisode.EXPECT().Tx(tx).Return(defaultEpisodeTx).Once()
	defaultEpisodeTx.EXPECT().
		GetActiveByCustomerID(testifymock.Anything, profileID).
		Return(model.DefaultEpisode{}, cerr.ResourceNotFound(errors.New("none"))).
		Once()

	installment := mock.NewInstallment(t)
	installmentTx := mock.NewInstallmentTxQueryer(t)
	installment.EXPECT().Tx(tx).Return(installmentTx).Once()
	installmentTx.EXPECT().
		ListCollectionsObligationsByCustomerID(testifymock.Anything, profileID).
		Return(nil, nil).
		Once()

	customerResolver := mock.NewCustomerResolver(t)
	expectSummaryResolveActive(t, customerResolver, externalCustomerID)

	uc := newSummaryUseCase(
		t,
		pool,
		customerProfile,
		creditAccount,
		policyAssignment,
		clk,
		nil,
		customerRestriction,
		installment,
		defaultEpisode,
		customerResolver,
	)

	out, err := uc.GetSummary(context.Background(), customercredituc.Input{
		SubjectID:  model.SubjectID(uuid.New()),
		MobileE164: summaryTestMobileE164,
	})
	require.NoError(t, err)
	require.Equal(t, customercredituc.SummaryStatusExpired, out.Status)
	require.Equal(t, uint64(0), out.AvailableAmountMinor)
	require.NotNil(t, out.LimitStatus)
	require.Equal(t, model.ExpiredCreditAccountStatus, *out.LimitStatus)
	require.NotEqual(t, customercredituc.SummaryStatusNotAssigned, out.Status)
}

func TestGetSummary_ActiveLimit_UsesInternalCustomerIDNotExternal(t *testing.T) {
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

	externalCustomerID := model.ExternalCustomerID(uuid.MustParse("11111111-1111-4111-8111-111111111111"))
	profileID := model.CustomerID(uuid.MustParse("22222222-2222-4222-8222-222222222222"))
	accountID := model.CreditAccountID(uuid.New())
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	effectiveFrom := now.Add(-24 * time.Hour)
	expiresAt := now.Add(30 * 24 * time.Hour)

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
			testifymock.Anything,
			externalCustomerID,
			model.IRJurisdiction,
			model.ActiveCustomerStatus,
			now,
			now,
		).
		Return(model.CustomerProfile{
			ID:                 profileID,
			ExternalCustomerID: externalCustomerID,
			UpdatedAt:          now,
		}, false, nil).
		Once()

	creditAccount.EXPECT().Tx(tx).Return(creditAccountTx).Once()
	expectSummaryPolicyAssignmentDefault(t, tx, policyAssignment)
	creditAccountTx.EXPECT().
		ExpireDueByCustomerIDAndCurrency(testifymock.Anything, profileID, model.IRRCurrency, now).
		Return(nil).
		Once()

	activeAccount := model.CreditAccount{
		ID:                  accountID,
		CustomerID:          profileID,
		LimitAmountMinor:    90_000_000,
		ExposureAmountMinor: 10_000_000,
		Currency:            model.IRRCurrency,
		Status:              model.ActiveCreditAccountStatus,
		EffectiveFrom:       effectiveFrom,
		ExpiresAt:           &expiresAt,
		UpdatedAt:           now,
	}

	creditAccountTx.EXPECT().
		GetByCustomerIDAndCurrency(testifymock.Anything, profileID, model.IRRCurrency).
		Return(activeAccount, nil).
		Once()

	customerRestriction := mock.NewCustomerRestriction(t)
	customerRestrictionTx := mock.NewCustomerRestrictionTxQueryer(t)
	customerRestriction.EXPECT().Tx(tx).Return(customerRestrictionTx).Once()
	customerRestrictionTx.EXPECT().
		ListActiveByCustomerID(testifymock.Anything, profileID).
		Return(nil, nil).
		Once()

	defaultEpisode := mock.NewDefaultEpisode(t)
	defaultEpisodeTx := mock.NewDefaultEpisodeTxQueryer(t)
	defaultEpisode.EXPECT().Tx(tx).Return(defaultEpisodeTx).Once()
	defaultEpisodeTx.EXPECT().
		GetActiveByCustomerID(testifymock.Anything, profileID).
		Return(model.DefaultEpisode{}, cerr.ResourceNotFound(errors.New("none"))).
		Once()

	installment := mock.NewInstallment(t)
	installmentTx := mock.NewInstallmentTxQueryer(t)
	installment.EXPECT().Tx(tx).Return(installmentTx).Once()
	installmentTx.EXPECT().
		ListCollectionsObligationsByCustomerID(testifymock.Anything, profileID).
		Return(nil, nil).
		Once()

	customerResolver := mock.NewCustomerResolver(t)
	expectSummaryResolveActive(t, customerResolver, externalCustomerID)

	uc := newSummaryUseCase(
		t,
		pool,
		customerProfile,
		creditAccount,
		policyAssignment,
		clk,
		nil,
		customerRestriction,
		installment,
		defaultEpisode,
		customerResolver,
	)

	out, err := uc.GetSummary(context.Background(), customercredituc.Input{
		SubjectID:  model.SubjectID(uuid.New()),
		MobileE164: summaryTestMobileE164,
	})
	require.NoError(t, err)
	require.Equal(t, customercredituc.SummaryStatusActive, out.Status)
	require.Equal(t, profileID, out.CustomerID)
	require.Equal(t, uint64(90_000_000), out.LimitAmountMinor)
	require.Equal(t, uint64(80_000_000), out.AvailableAmountMinor)
	require.NotEqual(t, customercredituc.SummaryStatusNotAssigned, out.Status)
	require.NotEqual(t, model.CustomerID(externalCustomerID), out.CustomerID)
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
	now := time.Date(
		2026,
		8,
		9,
		12,
		0,
		0,
		0,
		time.UTC,
	)

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
			testifymock.Anything,
			externalCustomerID,
			model.IRJurisdiction,
			model.ActiveCustomerStatus,
			now,
			now,
		).
		Return(model.CustomerProfile{
			ID:                 profileID,
			ExternalCustomerID: externalCustomerID,
			UpdatedAt:          now,
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

	customerResolver := mock.NewCustomerResolver(t)
	expectSummaryResolveActive(t, customerResolver, externalCustomerID)

	uc := newSummaryUseCase(
		t,
		pool,
		customerProfile,
		creditAccount,
		policyAssignment,
		clk,
		nil,
		nil,
		nil,
		nil,
		customerResolver,
	)

	out, err := uc.GetSummary(context.Background(), customercredituc.Input{
		SubjectID:  model.SubjectID(uuid.New()),
		MobileE164: summaryTestMobileE164,
	})
	require.NoError(t, err)
	require.Equal(t, customercredituc.SummaryStatusNotAssigned, out.Status)
	require.Equal(t, uint64(0), out.AvailableAmountMinor)
	require.Nil(t, out.LimitStatus)
}
