package collectionsuc_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/policy/collections"
	collectionsv1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/collectionsuc"
)

func newCollectionsUseCase(
	t *testing.T,
	pool *mock.Pool,
	customerProfile *mock.CustomerProfile,
	installment *mock.Installment,
	decision *mock.Decision,
	defaultEpisode *mock.DefaultEpisode,
	customerRestriction *mock.CustomerRestriction,
	collectionsCase *mock.CollectionsCase,
	outbox *mock.Outbox,
	clk *mock.Clock,
) *collectionsuc.UseCase {
	t.Helper()

	uc, err := collectionsuc.New(
		pool,
		customerProfile,
		installment,
		decision,
		defaultEpisode,
		customerRestriction,
		collectionsCase,
		outbox,
		clk,
		mock.NewIdentifierGenerator(t),
		sha256fingerprint.New(),
		metricsnoop.New(),
		"bnpl.audit.events.v1",
		10, testPolicies(),
	)
	require.NoError(t, err)

	return uc
}

func TestAssertNewCreditAllowed_LiveDefaultDoesNotMutateCollections(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	tx := mock.NewTx(t)
	clk := mock.NewClock(t)
	customerProfile := mock.NewCustomerProfile(t)
	installment := mock.NewInstallment(t)
	decision := mock.NewDecision(t)
	defaultEpisode := mock.NewDefaultEpisode(t)
	customerRestriction := mock.NewCustomerRestriction(t)
	collectionsCase := mock.NewCollectionsCase(t)
	outbox := mock.NewOutbox(t)

	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

	restrictionTx := mock.NewCustomerRestrictionTxQueryer(t)
	customerRestriction.EXPECT().Tx(tx).Return(restrictionTx).Once()
	restrictionTx.EXPECT().
		ListActiveByCustomerID(testifymock.Anything, customerID).
		Return(nil, nil).
		Once()

	episodeTx := mock.NewDefaultEpisodeTxQueryer(t)
	defaultEpisode.EXPECT().Tx(tx).Return(episodeTx).Once()
	episodeTx.EXPECT().
		GetActiveByCustomerID(testifymock.Anything, customerID).
		Return(model.DefaultEpisode{}, cerr.ResourceNotFound(errors.New("none"))).
		Once()

	installmentTx := mock.NewInstallmentTxQueryer(t)
	installment.EXPECT().Tx(tx).Return(installmentTx).Once()
	installmentTx.EXPECT().
		ListCollectionsObligationsByCustomerID(testifymock.Anything, customerID).
		Return([]repo.CollectionsObligation{{
			Installment: model.Installment{
				ID:              installmentID,
				ContractID:      &contractID,
				CustomerID:      customerID,
				DueAt:           now.Add(-72 * time.Hour),
				GracePeriodDays: 1,
				Status:          model.OverdueInstallmentStatus,
			},
			HasUnresolvedPayment: false,
		}}, nil).
		Once()

	// Decision / episode / restriction create paths must not be touched.
	decision.AssertNotCalled(t, "Tx", testifymock.Anything)
	collectionsCase.AssertNotCalled(t, "Tx", testifymock.Anything)
	outbox.AssertNotCalled(t, "Tx", testifymock.Anything)

	uc := newCollectionsUseCase(
		t,
		pool,
		customerProfile,
		installment,
		decision,
		defaultEpisode,
		customerRestriction,
		collectionsCase,
		outbox,
		clk,
	)

	err := uc.AssertNewCreditAllowed(context.Background(), tx, customerID, now)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}

func TestAssessBatchOutcomeLabel(t *testing.T) {
	t.Parallel()

	require.Equal(t, "success", collectionsuc.AssessBatchOutcomeLabel(0, 0))
	require.Equal(t, "success", collectionsuc.AssessBatchOutcomeLabel(3, 0))
	require.Equal(t, "partial_failure", collectionsuc.AssessBatchOutcomeLabel(3, 1))
	require.Equal(t, "failure", collectionsuc.AssessBatchOutcomeLabel(2, 2))
}

func testPolicies() *collections.Registry {
	r, _ := collections.NewRegistry(collectionsv1.New())
	return r
}
