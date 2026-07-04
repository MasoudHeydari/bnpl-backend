package contractuc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/event/audit"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

type cancelDeps struct {
	pool                 *mock.Pool
	conn                 *mock.Conn
	tx                   *mock.Tx
	customerProfile      *mock.CustomerProfile
	customerProfileTx    *mock.CustomerProfileTxQueryer
	contract             *mock.Contract
	contractTx           *mock.ContractTxQueryer
	paymentAttempt       *mock.PaymentAttempt
	paymentAttemptTx     *mock.PaymentAttemptTxQueryer
	installment          *mock.Installment
	installmentTx        *mock.InstallmentTxQueryer
	outbox               *mock.Outbox
	outboxTx             *mock.OutboxTxQueryer
	checkoutPayment      *mock.CheckoutPaymentClient
	contractCancellation *mock.ContractCancellationClient
	saga                 *mock.Saga
	sagaTx               *mock.SagaTxQueryer
	sagaConn             *mock.SagaConnQueryer
	executor             *mock.Executor
	idGenerator          *mock.IdentifierGenerator
	clock                adapterclock.Fixed
}

func newCancelDeps(t *testing.T, now time.Time) cancelDeps {
	t.Helper()

	return cancelDeps{
		pool:                 mock.NewPool(t),
		conn:                 mock.NewConn(t),
		tx:                   mock.NewTx(t),
		customerProfile:      mock.NewCustomerProfile(t),
		customerProfileTx:    mock.NewCustomerProfileTxQueryer(t),
		contract:             mock.NewContract(t),
		contractTx:           mock.NewContractTxQueryer(t),
		paymentAttempt:       mock.NewPaymentAttempt(t),
		paymentAttemptTx:     mock.NewPaymentAttemptTxQueryer(t),
		installment:          mock.NewInstallment(t),
		installmentTx:        mock.NewInstallmentTxQueryer(t),
		outbox:               mock.NewOutbox(t),
		outboxTx:             mock.NewOutboxTxQueryer(t),
		checkoutPayment:      mock.NewCheckoutPaymentClient(t),
		contractCancellation: mock.NewContractCancellationClient(t),
		saga:                 mock.NewSaga(t),
		sagaTx:               mock.NewSagaTxQueryer(t),
		sagaConn:             mock.NewSagaConnQueryer(t),
		executor:             mock.NewExecutor(t),
		idGenerator:          mock.NewIdentifierGenerator(t),
		clock:                adapterclock.NewFixed(now, time.UTC),
	}
}

func (d cancelDeps) newUseCase(t *testing.T) *UseCase {
	t.Helper()

	settings := testCancellationSagaSettings()
	definition, err := NewCancellationSagaDefinition(
		d.pool,
		d.contract,
		mock.NewCreditAccount(t),
		d.contractCancellation,
		d.clock,
		settings.RetryBaseDelay,
		settings.RetryMaxDelay,
	)
	require.NoError(t, err)

	uc, err := New(
		d.pool,
		d.customerProfile,
		mock.NewMerchantProfile(t),
		mock.NewCreditAccount(t),
		mock.NewDecision(t),
		d.contract,
		mock.NewCheckoutDecisionAllocation(t),
		mustTestCheckoutRegistry(t),
		d.installment,
		mock.NewInstallmentCharge(t),
		d.paymentAttempt,
		d.checkoutPayment,
		d.contractCancellation,
		d.outbox,
		d.clock,
		"audit-events",
		d.idGenerator,
		"https://bnpl.example/payments/return",
		5*time.Minute,
		100,
		settings,
		d.saga,
		d.executor,
		definition,
		mustTestOriginationJurisdictions(t),
		lognoop.New(),
		metricsnoop.New(),
		AllowAllNewCreditUsage{},
	)
	if err != nil {
		panic(err)
	}

	return uc
}

func (d cancelDeps) expectConnTx(ctx context.Context) {
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
	d.customerProfile.EXPECT().Tx(d.tx).Return(d.customerProfileTx).Once()
	d.contract.EXPECT().Tx(d.tx).Return(d.contractTx).Once()
	d.paymentAttempt.EXPECT().Tx(d.tx).Return(d.paymentAttemptTx).Once()
	d.outbox.EXPECT().Tx(d.tx).Return(d.outboxTx).Once()
}

func (d cancelDeps) expectStartCancellationSaga(
	ctx context.Context,
	contractID model.ContractID,
	canceling model.Contract,
	now time.Time,
	sagaID model.SagaID,
) {
	d.saga.EXPECT().Tx(d.tx).Return(d.sagaTx).Once()
	d.sagaTx.EXPECT().Create(
		ctx,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		now,
		now,
	).RunAndReturn(func(
		_ context.Context,
		sagaType model.SagaType,
		definitionVersion model.SagaDefinitionVersion,
		businessKey model.SagaBusinessKey,
		subjectType *string,
		subjectID *string,
		phase model.SagaPhase,
		status model.SagaStatus,
		currentStep model.SagaStep,
		_ *time.Time,
		correlationID *string,
		causationID *string,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.SagaInstance, error) {
		return model.SagaInstance{
			ID:                sagaID,
			Type:              sagaType,
			DefinitionVersion: definitionVersion,
			BusinessKey:       businessKey,
			SubjectType:       subjectType,
			SubjectID:         subjectID,
			Phase:             phase,
			Status:            status,
			CurrentStep:       currentStep,
			Version:           1,
			CorrelationID:     correlationID,
			CausationID:       causationID,
			CreatedAt:         createdAt,
			UpdatedAt:         updatedAt,
		}, nil
	}).Once()

	d.contract.EXPECT().Tx(d.tx).Return(d.contractTx).Twice()
	d.contractTx.EXPECT().LockByID(ctx, contractID).Return(canceling, nil).Once()
	d.sagaTx.EXPECT().Update(
		ctx,
		sagaID,
		int64(1),
		model.ForwardSagaPhase,
		model.RunningSagaStatus,
		contractCancellationLocalStartedStep,
		1,
		(*time.Time)(nil),
		(*string)(nil),
		(*time.Time)(nil),
		now,
	).Return(model.SagaInstance{
		ID:               sagaID,
		CurrentStep:      contractCancellationLocalStartedStep,
		StepAttemptCount: 1,
		Version:          2,
	}, nil).Once()
	d.sagaTx.EXPECT().RecordForwardStepCompletion(
		ctx,
		sagaID,
		contractCancellationLocalStartedStep,
		1,
		now,
	).Return(model.SagaForwardStepCompletion{}, nil).Once()
	d.sagaTx.EXPECT().Update(
		ctx,
		sagaID,
		int64(2),
		model.ForwardSagaPhase,
		model.RunningSagaStatus,
		contractCancellationExternalStep,
		0,
		(*time.Time)(nil),
		(*string)(nil),
		(*time.Time)(nil),
		now,
	).Return(model.SagaInstance{ID: sagaID}, nil).Once()
}

// expectFinalizeLeavesCanceling mocks the post-commit finalize probe when the
// Saga has not yet reached the local canceling → canceled finalizer step.
func (d cancelDeps) expectFinalizeLeavesCanceling(
	ctx context.Context,
	sagaID model.SagaID,
) {
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
	d.saga.EXPECT().Conn(d.conn).Return(d.sagaConn).Once()
	d.sagaConn.EXPECT().GetByID(ctx, sagaID).Return(model.SagaInstance{
		ID:          sagaID,
		Phase:       model.ForwardSagaPhase,
		Status:      model.RunningSagaStatus,
		CurrentStep: contractCancellationExternalStep,
	}, nil).Once()
}

func cancelTraceContext() context.Context {
	ctx := context.Background()
	ctx = trace.WithTraceID(ctx, "trace-cancel")
	ctx = trace.WithCorrelationID(ctx, "corr-cancel")
	return ctx
}

func activeContractFixture(
	contractID model.ContractID,
	customerID model.CustomerID,
	remaining uint64,
) model.Contract {
	return model.Contract{
		ID:                            contractID,
		CustomerID:                    customerID,
		Status:                        model.ActiveContractStatus,
		RemainingPrincipalAmountMinor: remaining,
		Currency:                      model.IRRCurrency,
	}
}

func TestCancel_ActiveToCanceling(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		9,
		5,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCancelDeps(t, now)
	uc := deps.newUseCase(t)
	ctx := cancelTraceContext()

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	tenantID := model.TenantID(uuid.New())
	contextID := model.ContextID(uuid.New())
	idempotencyKey := "cancel-key-1"
	remaining := uint64(1_500_000)

	active := activeContractFixture(contractID, customerID, remaining)
	canceling := active
	canceling.Status = model.CancelingContractStatus
	key := idempotencyKey
	reason := model.CustomerRequestedContractCancellationReason
	fp := cancellationRequestFingerprint(
		contractID,
		customerID,
		reason,
		tenantID,
		contextID,
	)
	fcKey := financialCoreCancellationIdempotencyKey(contractID)
	canceling.CancellationIdempotencyKey = &key
	canceling.CancellationReason = &reason
	canceling.CancellationRequestFingerprint = &fp
	canceling.CancellationRequestedAt = &now
	canceling.CancellationFinancialCoreIdempotencyKey = &fcKey
	canceling.CancellationTenantID = &tenantID
	canceling.CancellationContextID = &contextID

	deps.expectConnTx(ctx)
	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.contractTx.EXPECT().LockByID(ctx, contractID).Return(active, nil).Once()
	deps.paymentAttemptTx.EXPECT().
		HasBlockingAttemptForContract(ctx, contractID).
		Return(false, nil).
		Once()
	deps.installment.EXPECT().Tx(deps.tx).Return(deps.installmentTx).Once()
	deps.installmentTx.EXPECT().LockByContractID(ctx, contractID).Return(nil, nil).Once()

	sagaID := model.SagaID(uuid.New())
	deps.expectStartCancellationSaga(
		ctx,
		contractID,
		canceling,
		now,
		sagaID,
	)
	deps.contractTx.EXPECT().MarkCanceling(
		ctx,
		contractID,
		idempotencyKey,
		reason,
		fp,
		fcKey,
		tenantID,
		contextID,
		now,
		now,
	).RunAndReturn(func(
		context.Context,
		model.ContractID,
		string,
		model.ContractCancellationReason,
		string,
		string,
		model.TenantID,
		model.ContextID,
		time.Time,
		time.Time,
	) (model.Contract, error) {
		return canceling, nil
	}).Once()
	deps.idGenerator.EXPECT().Generate().Return("audit-event-1").Once()
	deps.outboxTx.EXPECT().Create(ctx, testifymock.Anything).RunAndReturn(
		func(_ context.Context, msg repo.CreateOutboxMessage) (model.OutboxMessage, error) {
			require.Equal(t, "audit-events", msg.Destination)
			require.Equal(t, contractID.String(), msg.MessageKey)
			require.Equal(t, audit.EventType, msg.EventType)

			var event audit.Event
			require.NoError(t, json.Unmarshal(msg.Payload, &event))
			require.Equal(t, cancelContractAuditAction, event.Data.Action)
			require.Equal(t, string(model.ActiveContractStatus), fmt.Sprint(event.Data.Metadata["previous_status"]))
			require.Equal(t, string(model.CancelingContractStatus), fmt.Sprint(event.Data.Metadata["new_status"]))
			require.Equal(t, reason.String(), fmt.Sprint(event.Data.Metadata["reason"]))
			require.EqualValues(t, remaining, event.Data.Metadata["remaining_principal_amount_minor"])
			return model.OutboxMessage{ID: model.OutboxID(uuid.New())}, nil
		},
	).Once()
	deps.executor.EXPECT().ExecuteByID(ctx, sagaID).Return(nil).Once()
	deps.expectFinalizeLeavesCanceling(ctx, sagaID)

	output, err := uc.Cancel(ctx, CancelInput{
		TenantID:           tenantID,
		ContextID:          contextID,
		ContractID:         contractID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     idempotencyKey,
		Reason:             reason,
	})
	require.NoError(t, err)
	require.Equal(t, CancelResultStarted, output.Result)
	require.Equal(t, model.CancelingContractStatus, output.Contract.Status)
	require.Equal(t, remaining, output.Contract.RemainingPrincipalAmountMinor)
}

func TestCancel_SagaExecuteByIDFailureReturnsUnavailable(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		9,
		5,
		12,
		5,
		0,
		0,
		time.UTC,
	)
	deps := newCancelDeps(t, now)
	uc := deps.newUseCase(t)
	ctx := cancelTraceContext()

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	tenantID := model.TenantID(uuid.New())
	contextID := model.ContextID(uuid.New())
	idempotencyKey := "cancel-key-fc-failure"
	reason := model.CustomerRequestedContractCancellationReason
	fp := cancellationRequestFingerprint(
		contractID,
		customerID,
		reason,
		tenantID,
		contextID,
	)
	fcKey := financialCoreCancellationIdempotencyKey(contractID)

	active := activeContractFixture(contractID, customerID, 500_000)
	canceling := active
	canceling.Status = model.CancelingContractStatus
	canceling.CancellationIdempotencyKey = &idempotencyKey
	canceling.CancellationReason = &reason
	canceling.CancellationRequestFingerprint = &fp
	canceling.CancellationRequestedAt = &now
	canceling.CancellationFinancialCoreIdempotencyKey = &fcKey
	canceling.CancellationTenantID = &tenantID
	canceling.CancellationContextID = &contextID

	deps.expectConnTx(ctx)
	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.contractTx.EXPECT().LockByID(ctx, contractID).Return(active, nil).Once()
	deps.paymentAttemptTx.EXPECT().
		HasBlockingAttemptForContract(ctx, contractID).
		Return(false, nil).
		Once()
	deps.installment.EXPECT().Tx(deps.tx).Return(deps.installmentTx).Once()
	deps.installmentTx.EXPECT().LockByContractID(ctx, contractID).Return(nil, nil).Once()

	sagaID := model.SagaID(uuid.New())
	deps.expectStartCancellationSaga(
		ctx,
		contractID,
		canceling,
		now,
		sagaID,
	)
	deps.contractTx.EXPECT().MarkCanceling(
		ctx,
		contractID,
		idempotencyKey,
		reason,
		fp,
		fcKey,
		tenantID,
		contextID,
		now,
		now,
	).Return(canceling, nil).Once()
	deps.idGenerator.EXPECT().Generate().Return("audit-event-fc-failure").Once()
	deps.outboxTx.EXPECT().Create(ctx, testifymock.Anything).
		Return(model.OutboxMessage{ID: model.OutboxID(uuid.New())}, nil).
		Once()

	fcErr := errors.New("financial core unavailable")
	deps.executor.EXPECT().ExecuteByID(ctx, sagaID).Return(fcErr).Once()

	_, err := uc.Cancel(ctx, CancelInput{
		TenantID:           tenantID,
		ContextID:          contextID,
		ContractID:         contractID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     idempotencyKey,
		Reason:             reason,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeServiceUnavailable))
}

func TestCancel_ExactIdempotentReplay(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		9,
		5,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCancelDeps(t, now)
	uc := deps.newUseCase(t)
	ctx := cancelTraceContext()

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	tenantID := model.TenantID(uuid.New())
	contextID := model.ContextID(uuid.New())
	idempotencyKey := "cancel-key-replay"
	reason := model.CustomerRequestedContractCancellationReason
	fp := cancellationRequestFingerprint(
		contractID,
		customerID,
		reason,
		tenantID,
		contextID,
	)
	fcKey := financialCoreCancellationIdempotencyKey(contractID)
	requestedAt := now.Add(-time.Minute)

	canceling := activeContractFixture(contractID, customerID, 900_000)
	canceling.Status = model.CancelingContractStatus
	canceling.CancellationIdempotencyKey = &idempotencyKey
	canceling.CancellationReason = &reason
	canceling.CancellationRequestFingerprint = &fp
	canceling.CancellationRequestedAt = &requestedAt
	canceling.CancellationFinancialCoreIdempotencyKey = &fcKey

	deps.expectConnTx(ctx)
	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.contractTx.EXPECT().LockByID(ctx, contractID).Return(canceling, nil).Once()

	output, err := uc.Cancel(ctx, CancelInput{
		TenantID:           tenantID,
		ContextID:          contextID,
		ContractID:         contractID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     idempotencyKey,
		Reason:             reason,
	})
	require.NoError(t, err)
	require.Equal(t, CancelResultAlreadyCanceling, output.Result)
	require.Equal(t, model.CancelingContractStatus, output.Contract.Status)
}

func TestCancel_IdempotencyConflictDifferentReason(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		9,
		5,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCancelDeps(t, now)
	uc := deps.newUseCase(t)
	ctx := cancelTraceContext()

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	tenantID := model.TenantID(uuid.New())
	contextID := model.ContextID(uuid.New())
	idempotencyKey := "same-key"
	storedReason := model.CustomerRequestedContractCancellationReason
	storedFP := cancellationRequestFingerprint(
		contractID,
		customerID,
		storedReason,
		tenantID,
		contextID,
	)

	canceling := activeContractFixture(contractID, customerID, 100)
	canceling.Status = model.CancelingContractStatus
	canceling.CancellationIdempotencyKey = &idempotencyKey
	canceling.CancellationReason = &storedReason
	canceling.CancellationRequestFingerprint = &storedFP

	deps.expectConnTx(ctx)
	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.contractTx.EXPECT().LockByID(ctx, contractID).Return(canceling, nil).Once()

	// Same key but different tenant/context changes the fingerprint.
	_, err := uc.Cancel(ctx, CancelInput{
		TenantID:           model.TenantID(uuid.New()),
		ContextID:          contextID,
		ContractID:         contractID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     idempotencyKey,
		Reason:             storedReason,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeIdempotencyConflict))
}

func TestCancel_OwnershipForbidden(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		9,
		5,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCancelDeps(t, now)
	uc := deps.newUseCase(t)
	ctx := cancelTraceContext()

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	otherCustomerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())

	deps.expectConnTx(ctx)
	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.contractTx.EXPECT().
		LockByID(ctx, contractID).
		Return(activeContractFixture(contractID, otherCustomerID, 1), nil).
		Once()

	_, err := uc.Cancel(ctx, CancelInput{
		TenantID:           model.TenantID(uuid.New()),
		ContextID:          model.ContextID(uuid.New()),
		ContractID:         contractID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     "k",
		Reason:             model.CustomerRequestedContractCancellationReason,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeForbidden))
}

func TestCancel_RejectsNonActiveStates(t *testing.T) {
	t.Parallel()

	cases := []model.ContractStatus{
		model.PaymentPendingContractStatus,
		model.CompletedContractStatus,
		model.DefaultedContractStatus,
		model.ExpiredContractStatus,
	}

	for _, status := range cases {
		status := status
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			now := time.Date(
				2026,
				9,
				5,
				12,
				0,
				0,
				0,
				time.UTC,
			)
			deps := newCancelDeps(t, now)
			uc := deps.newUseCase(t)
			ctx := cancelTraceContext()

			externalCustomerID := model.ExternalCustomerID(uuid.New())
			customerID := model.CustomerID(uuid.New())
			contractID := model.ContractID(uuid.New())
			row := activeContractFixture(contractID, customerID, 1)
			row.Status = status

			deps.expectConnTx(ctx)
			deps.customerProfileTx.EXPECT().
				LockByExternalCustomerID(ctx, externalCustomerID).
				Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
				Once()
			deps.contractTx.EXPECT().LockByID(ctx, contractID).Return(row, nil).Once()

			_, err := uc.Cancel(ctx, CancelInput{
				TenantID:           model.TenantID(uuid.New()),
				ContextID:          model.ContextID(uuid.New()),
				ContractID:         contractID,
				ExternalCustomerID: externalCustomerID,
				IdempotencyKey:     "k",
				Reason:             model.CustomerRequestedContractCancellationReason,
			})
			require.Error(t, err)
			require.True(t, cerr.IsCode(err, cerr.CodeConflict))
		})
	}
}

func TestCancel_CanceledWithoutMatchingKey_Conflicts(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		9,
		5,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCancelDeps(t, now)
	uc := deps.newUseCase(t)
	ctx := cancelTraceContext()

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	row := activeContractFixture(contractID, customerID, 1)
	row.Status = model.CanceledContractStatus

	deps.expectConnTx(ctx)
	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.contractTx.EXPECT().LockByID(ctx, contractID).Return(row, nil).Once()

	_, err := uc.Cancel(ctx, CancelInput{
		TenantID:           model.TenantID(uuid.New()),
		ContextID:          model.ContextID(uuid.New()),
		ContractID:         contractID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     "new-key",
		Reason:             model.CustomerRequestedContractCancellationReason,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}

func TestCancel_SecondKeyWhileCanceling_Conflicts(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		9,
		5,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCancelDeps(t, now)
	uc := deps.newUseCase(t)
	ctx := cancelTraceContext()

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	tenantID := model.TenantID(uuid.New())
	contextID := model.ContextID(uuid.New())
	storedKey := "first-key"
	reason := model.CustomerRequestedContractCancellationReason
	fp := cancellationRequestFingerprint(
		contractID,
		customerID,
		reason,
		tenantID,
		contextID,
	)

	canceling := activeContractFixture(contractID, customerID, 1)
	canceling.Status = model.CancelingContractStatus
	canceling.CancellationIdempotencyKey = &storedKey
	canceling.CancellationReason = &reason
	canceling.CancellationRequestFingerprint = &fp

	deps.expectConnTx(ctx)
	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.contractTx.EXPECT().LockByID(ctx, contractID).Return(canceling, nil).Once()

	_, err := uc.Cancel(ctx, CancelInput{
		TenantID:           tenantID,
		ContextID:          contextID,
		ContractID:         contractID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     "other-key",
		Reason:             reason,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}

func TestContractStatus_ActiveCannotTransitionDirectlyToCanceled(t *testing.T) {
	t.Parallel()

	require.False(t, model.ActiveContractStatus.CanTransitionTo(model.CanceledContractStatus))
	require.True(t, model.ActiveContractStatus.CanTransitionTo(model.CancelingContractStatus))
	require.True(t, model.CancelingContractStatus.CanTransitionTo(model.CanceledContractStatus))
	require.True(t, model.CancelingContractStatus.IsValid())
	require.False(t, model.CancelingContractStatus.IsTerminal())
}

func TestCancel_BlockedByInFlightPaymentAttempt(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		9,
		6,
		10,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newCancelDeps(t, now)
	uc := deps.newUseCase(t)
	ctx := cancelTraceContext()

	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())
	active := activeContractFixture(contractID, customerID, 500_000)

	deps.expectConnTx(ctx)
	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.contractTx.EXPECT().LockByID(ctx, contractID).Return(active, nil).Once()
	deps.paymentAttemptTx.EXPECT().
		HasBlockingAttemptForContract(ctx, contractID).
		Return(true, nil).
		Once()

	_, err := uc.Cancel(ctx, CancelInput{
		TenantID:           model.TenantID(uuid.New()),
		ContextID:          model.ContextID(uuid.New()),
		ContractID:         contractID,
		ExternalCustomerID: externalCustomerID,
		IdempotencyKey:     "cancel-with-payment-in-flight",
		Reason:             model.CustomerRequestedContractCancellationReason,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}
