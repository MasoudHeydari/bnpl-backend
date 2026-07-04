package contractuc

import (
	"context"
	"errors"
	"testing"
	"time"

		lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type expirePaymentPendingDeps struct {
	pool                       *mock.Pool
	conn                       *mock.Conn
	tx                         *mock.Tx
	customerProfile            *mock.CustomerProfile
	creditAccount              *mock.CreditAccount
	decision                   *mock.Decision
	decisionTx                 *mock.DecisionTxQueryer
	contract                   *mock.Contract
	contractTx                 *mock.ContractTxQueryer
	installment                *mock.Installment
	installmentTx              *mock.InstallmentTxQueryer
	installmentCharge          *mock.InstallmentCharge
	installmentChargeTx        *mock.InstallmentChargeTxQueryer
	paymentAttempt             *mock.PaymentAttempt
	paymentAttemptTx           *mock.PaymentAttemptTxQueryer
	checkoutDecisionAllocation *mock.CheckoutDecisionAllocation
	checkoutPayment            *mock.CheckoutPaymentClient
	contractCancellation       *mock.ContractCancellationClient
	inbox                      *mock.Inbox
	outbox                     *mock.Outbox
	outboxTx                   *mock.OutboxTxQueryer
	idGenerator                *mock.IdentifierGenerator
	clock                      adapterclock.Fixed
}

func newExpirePaymentPendingDeps(t *testing.T, now time.Time) expirePaymentPendingDeps {
	t.Helper()

	return expirePaymentPendingDeps{
		pool:                       mock.NewPool(t),
		conn:                       mock.NewConn(t),
		tx:                         mock.NewTx(t),
		customerProfile:            mock.NewCustomerProfile(t),
		creditAccount:              mock.NewCreditAccount(t),
		decision:                   mock.NewDecision(t),
		decisionTx:                 mock.NewDecisionTxQueryer(t),
		contract:                   mock.NewContract(t),
		contractTx:                 mock.NewContractTxQueryer(t),
		installment:                mock.NewInstallment(t),
		installmentTx:              mock.NewInstallmentTxQueryer(t),
		installmentCharge:          mock.NewInstallmentCharge(t),
		installmentChargeTx:        mock.NewInstallmentChargeTxQueryer(t),
		paymentAttempt:             mock.NewPaymentAttempt(t),
		paymentAttemptTx:           mock.NewPaymentAttemptTxQueryer(t),
		checkoutDecisionAllocation: mock.NewCheckoutDecisionAllocation(t),
		checkoutPayment:            mock.NewCheckoutPaymentClient(t),
		contractCancellation:       mock.NewContractCancellationClient(t),
		inbox:                      mock.NewInbox(t),
		outbox:                     mock.NewOutbox(t),
		outboxTx:                   mock.NewOutboxTxQueryer(t),
		idGenerator:                mock.NewIdentifierGenerator(t),
		clock:                      adapterclock.NewFixed(now, time.UTC),
	}
}

func (d expirePaymentPendingDeps) newUseCase(t *testing.T, grace time.Duration, batchSize int) *UseCase {
	t.Helper()

	sagaRepo, sagaExecutor, sagaDefinition := mustTestCancellationSagaDeps(
		t,
		d.pool,
		d.contract,
		d.creditAccount,
		d.contractCancellation,
		d.clock,
	)

	uc, err := New(
		d.pool,
		d.customerProfile,
		mock.NewMerchantProfile(t),
		d.creditAccount,
		d.decision,
		d.contract,
		d.checkoutDecisionAllocation,
		mustTestCheckoutRegistry(t),
		d.installment,
		d.installmentCharge,
		d.paymentAttempt,
		d.checkoutPayment,
		d.contractCancellation,
		d.outbox,
		d.clock,
		"audit-events",
		d.idGenerator,
		"https://bnpl.example/payments/return",
		grace,
		batchSize,
		testCancellationSagaSettings(),
		sagaRepo,
		sagaExecutor,
		sagaDefinition,
		mustTestOriginationJurisdictions(t),
		lognoop.New(),
		metricsnoop.New(),
	)
	if err != nil {
		panic(err)
	}

	return uc
}

func (d expirePaymentPendingDeps) expectConnTx(ctx context.Context) {
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
	d.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, d.tx)
		},
	).Once()
	d.contract.EXPECT().Tx(d.tx).Return(d.contractTx).Once()
}

func (d expirePaymentPendingDeps) expectExpireConnTx(ctx context.Context) {
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
	d.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, d.tx)
		},
	).Once()
	d.contract.EXPECT().Tx(d.tx).Return(d.contractTx).Once()
	d.installment.EXPECT().Tx(d.tx).Return(d.installmentTx).Once()
	d.installmentCharge.EXPECT().Tx(d.tx).Return(d.installmentChargeTx).Once()
	d.paymentAttempt.EXPECT().Tx(d.tx).Return(d.paymentAttemptTx).Once()
	d.decision.EXPECT().Tx(d.tx).Return(d.decisionTx).Once()
	d.outbox.EXPECT().Tx(d.tx).Return(d.outboxTx).Once()
}

func samplePaymentPendingContract(deadline time.Time) model.Contract {
	return model.Contract{
		ID:         model.ContractID(uuid.New()),
		DecisionID: model.DecisionID(uuid.New()),
		Status:     model.PaymentPendingContractStatus,
		ExpiresAt:  deadline,
	}
}

func TestExpireDuePaymentPendingContracts_NoEligibleContracts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newExpirePaymentPendingDeps(t, now)
	grace := 5 * time.Minute

	deps.expectConnTx(ctx)
	deps.contractTx.EXPECT().ListDuePaymentPendingContractIDs(ctx, now, grace, 100).Return(nil, nil).Once()

	uc := deps.newUseCase(t, grace, 100)

	output, err := uc.ExpireDuePaymentPendingContracts(ctx)
	require.NoError(t, err)
	require.Zero(t, output.ExpiredContractCount)
}

func TestExpireDuePaymentPendingContracts_ProcessingGraceNotElapsed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deadline := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	grace := 5 * time.Minute
	now := deadline.Add(4 * time.Minute)
	deps := newExpirePaymentPendingDeps(t, now)
	contractRow := samplePaymentPendingContract(deadline)

	deps.expectConnTx(ctx)
	deps.contractTx.EXPECT().ListDuePaymentPendingContractIDs(ctx, now, grace, 100).
		Return([]model.ContractID{contractRow.ID}, nil).Once()
	deps.expectExpireConnTx(ctx)
	deps.contractTx.EXPECT().LockByID(ctx, contractRow.ID).Return(contractRow, nil).Once()

	uc := deps.newUseCase(t, grace, 100)

	output, err := uc.ExpireDuePaymentPendingContracts(ctx)
	require.NoError(t, err)
	require.Zero(t, output.ExpiredContractCount)
}

func TestExpireDuePaymentPendingContracts_ExpiresContractAndRelatedRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deadline := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	grace := 5 * time.Minute
	now := deadline.Add(grace)
	deps := newExpirePaymentPendingDeps(t, now)
	contractRow := samplePaymentPendingContract(deadline)
	expiredContract := contractRow
	expiredContract.Status = model.ExpiredContractStatus

	deps.expectConnTx(ctx)
	deps.contractTx.EXPECT().ListDuePaymentPendingContractIDs(ctx, now, grace, 100).
		Return([]model.ContractID{contractRow.ID}, nil).Once()
	deps.expectExpireConnTx(ctx)
	deps.contractTx.EXPECT().LockByID(ctx, contractRow.ID).Return(contractRow, nil).Once()
	deps.contractTx.EXPECT().MarkExpired(ctx, contractRow.ID, now).Return(expiredContract, nil).Once()
	deps.paymentAttemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		ctx, contractRow.ID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(model.PaymentAttempt{}, cerr.ResourceNotFound(errors.New("payment attempt not found"))).Twice()
	deps.decisionTx.EXPECT().GetByID(ctx, contractRow.DecisionID).Return(model.Decision{
		ID:            contractRow.DecisionID,
		PolicyID:      "checkout_contract_approval",
		PolicyVersion: "v1",
	}, nil).Once()
	deps.installmentTx.EXPECT().ExpireOfferedByContractID(ctx, contractRow.ID, now).Return(int64(2), nil).Once()
	deps.installmentChargeTx.EXPECT().CancelOfferedByContractID(ctx, contractRow.ID, now).Return(int64(3), nil).Once()
	deps.idGenerator.EXPECT().Generate().Return("audit-event-id").Once()
	deps.outboxTx.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()

	uc := deps.newUseCase(t, grace, 100)

	output, err := uc.ExpireDuePaymentPendingContracts(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), output.ExpiredContractCount)
	require.Equal(t, int64(2), output.ExpiredInstallmentCount)
	require.Equal(t, int64(3), output.CanceledChargeCount)
}

func TestExpireDuePaymentPendingContracts_AlreadyActiveSkipped(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deadline := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	grace := 5 * time.Minute
	now := deadline.Add(grace)
	deps := newExpirePaymentPendingDeps(t, now)
	claimed := samplePaymentPendingContract(deadline)
	active := claimed
	active.Status = model.ActiveContractStatus

	deps.expectConnTx(ctx)
	deps.contractTx.EXPECT().ListDuePaymentPendingContractIDs(ctx, now, grace, 100).
		Return([]model.ContractID{claimed.ID}, nil).Once()
	deps.expectExpireConnTx(ctx)
	deps.contractTx.EXPECT().LockByID(ctx, claimed.ID).Return(active, nil).Once()

	uc := deps.newUseCase(t, grace, 100)

	output, err := uc.ExpireDuePaymentPendingContracts(ctx)
	require.NoError(t, err)
	require.Zero(t, output.ExpiredContractCount)
}

func TestExpireDuePaymentPendingContracts_MarkExpiredConflictIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deadline := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	grace := 5 * time.Minute
	now := deadline.Add(grace)
	deps := newExpirePaymentPendingDeps(t, now)
	contractRow := samplePaymentPendingContract(deadline)

	deps.expectConnTx(ctx)
	deps.contractTx.EXPECT().ListDuePaymentPendingContractIDs(ctx, now, grace, 100).
		Return([]model.ContractID{contractRow.ID}, nil).Once()
	deps.expectExpireConnTx(ctx)
	deps.contractTx.EXPECT().LockByID(ctx, contractRow.ID).Return(contractRow, nil).Once()
	deps.paymentAttemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		ctx, contractRow.ID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(model.PaymentAttempt{}, cerr.ResourceNotFound(errors.New("payment attempt not found"))).Once()
	deps.decisionTx.EXPECT().GetByID(ctx, contractRow.DecisionID).Return(model.Decision{
		ID:            contractRow.DecisionID,
		PolicyID:      "checkout_contract_approval",
		PolicyVersion: "v1",
	}, nil).Once()
	deps.contractTx.EXPECT().MarkExpired(ctx, contractRow.ID, now).Return(
		model.Contract{},
		cerr.Conflict(errors.New("already expired")),
	).Once()

	uc := deps.newUseCase(t, grace, 100)

	output, err := uc.ExpireDuePaymentPendingContracts(ctx)
	require.NoError(t, err)
	require.Zero(t, output.ExpiredContractCount)
}

func TestExpireDuePaymentPendingContracts_InstallmentUpdateFailureDoesNotRollbackOthers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deadline := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	grace := 5 * time.Minute
	now := deadline.Add(grace)
	deps := newExpirePaymentPendingDeps(t, now)
	contractRow := samplePaymentPendingContract(deadline)
	updateErr := errors.New("installment update failed")

	deps.expectConnTx(ctx)
	deps.contractTx.EXPECT().ListDuePaymentPendingContractIDs(ctx, now, grace, 100).
		Return([]model.ContractID{contractRow.ID}, nil).Once()
	deps.expectExpireConnTx(ctx)
	deps.contractTx.EXPECT().LockByID(ctx, contractRow.ID).Return(contractRow, nil).Once()
	deps.contractTx.EXPECT().MarkExpired(ctx, contractRow.ID, now).Return(contractRow, nil).Once()
	deps.paymentAttemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		ctx, contractRow.ID, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(model.PaymentAttempt{}, cerr.ResourceNotFound(errors.New("payment attempt not found"))).Twice()
	deps.decisionTx.EXPECT().GetByID(ctx, contractRow.DecisionID).Return(model.Decision{
		ID:            contractRow.DecisionID,
		PolicyID:      "checkout_contract_approval",
		PolicyVersion: "v1",
	}, nil).Once()
	deps.installmentTx.EXPECT().ExpireOfferedByContractID(ctx, contractRow.ID, now).Return(int64(0), updateErr).Once()

	uc := deps.newUseCase(t, grace, 100)

	_, err := uc.ExpireDuePaymentPendingContracts(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, updateErr)
}

func TestExpireDuePaymentPendingContracts_BatchLimitPassedToRepository(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newExpirePaymentPendingDeps(t, now)
	grace := 5 * time.Minute
	batchSize := 25

	deps.expectConnTx(ctx)
	deps.contractTx.EXPECT().ListDuePaymentPendingContractIDs(ctx, now, grace, batchSize).Return(nil, nil).Once()

	uc := deps.newUseCase(t, grace, batchSize)

	_, err := uc.ExpireDuePaymentPendingContracts(ctx)
	require.NoError(t, err)
}
