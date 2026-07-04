package checkoutsessionuc

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

// harnessSagaStore + harnessSagaExecutor give unit tests a durable saga identity
// without requiring every Confirm mock to model per-step Conn traffic. The
// executor collapses remaining forward work into the historical Confirm path
// (prepare → link → submit → optional merchant finalize). Production uses the
// real sagaexecutoruc against ConfirmationSagaDefinition steps.
type harnessSagaStore struct {
	mu        sync.Mutex
	instances map[model.SagaID]model.SagaInstance
	byKey     map[string]model.SagaID
}

func newHarnessSagaStore() *harnessSagaStore {
	return &harnessSagaStore{
		instances: make(map[model.SagaID]model.SagaInstance),
		byKey:     make(map[string]model.SagaID),
	}
}

func (s *harnessSagaStore) mapKey(t model.SagaType, b model.SagaBusinessKey) string {
	return string(t) + "|" + string(b)
}

type harnessSagaExecutor struct {
	store *harnessSagaStore
	uc    *UseCase
}

func (e *harnessSagaExecutor) ExecuteByID(ctx context.Context, id model.SagaID) error {
	if e.uc == nil {
		return nil
	}

	e.store.mu.Lock()
	instance, ok := e.store.instances[id]
	e.store.mu.Unlock()
	if !ok {
		return repo.ErrSagaNotRunnable
	}

	if instance.Status != model.RunningSagaStatus {
		return nil
	}

	sessionID, err := confirmationSagaSessionID(instance)
	if err != nil {
		return err
	}

	var session model.CheckoutSession
	err = e.uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			sessionQ := e.uc.checkoutSession.Tx(tx)
			row, getErr := sessionQ.LockByID(ctx, sessionID)
			if getErr != nil {
				return getErr
			}

			session = row
			return nil
		})
	})
	if err != nil {
		return err
	}

	if session.DecisionID == nil {
		return nil
	}

	externalCustomerID, err := e.loadExternalCustomerID(ctx, session)
	if err != nil {
		return err
	}

	created, err := e.uc.contractUC.PrepareCheckoutContract(ctx, contractuc.CreateContractInput{
		DecisionID:         *session.DecisionID,
		ExternalCustomerID: externalCustomerID,
	})
	if err != nil {
		if isDeterministicOfferNoLongerHonorable(err) {
			// Leave reopen/refresh to projectConfirmAfterSaga so unit tests keep
			// a single reopen expectation surface.
			e.markCompensated(id)
			return nil
		}

		return err
	}

	if _, err := e.uc.finishConfirm(ctx, session, created); err != nil {
		return err
	}

	created, err = e.uc.contractUC.SubmitPreparedCheckoutFulfillment(ctx, created)
	if err != nil {
		return err
	}

	if !created.PaymentRequired {
		if _, err := e.uc.completeRecognizedCheckout(ctx, created.Contract); err != nil {
			return err
		}
	}

	e.markCompleted(id)
	return nil
}

func (e *harnessSagaExecutor) loadExternalCustomerID(
	ctx context.Context,
	session model.CheckoutSession,
) (model.ExternalCustomerID, error) {
	if session.CustomerID == nil || session.CustomerID.IsZero() {
		return model.ExternalCustomerID{}, cerr.InternalError(nil)
	}

	var profile model.CustomerProfile
	err := e.uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		customerQ := e.uc.customerProfile.Conn(conn)
		row, getErr := customerQ.GetByID(ctx, *session.CustomerID)
		if getErr != nil {
			return getErr
		}

		profile = row
		return nil
	})
	if err != nil {
		return model.ExternalCustomerID{}, err
	}

	return profile.ExternalCustomerID, nil
}

func (e *harnessSagaExecutor) markCompleted(id model.SagaID) {
	e.store.mu.Lock()
	defer e.store.mu.Unlock()

	current := e.store.instances[id]
	current.Status = model.CompletedSagaStatus
	current.CurrentStep = checkoutPaymentSubmittedStep
	current.Version++
	e.store.instances[id] = current
}

func (e *harnessSagaExecutor) markCompensated(id model.SagaID) {
	e.store.mu.Lock()
	defer e.store.mu.Unlock()

	current := e.store.instances[id]
	current.Status = model.CompensatedSagaStatus
	current.Phase = model.CompensatingSagaPhase
	current.Version++
	e.store.instances[id] = current
}

func installHarnessSaga(h *harness) {
	h.t.Helper()

	store := newHarnessSagaStore()
	executor := &harnessSagaExecutor{store: store}

	sagaRepo := mock.NewSaga(h.t)
	txQ := mock.NewSagaTxQueryer(h.t)
	connQ := mock.NewSagaConnQueryer(h.t)

	sagaRepo.EXPECT().Tx(testifymock.Anything).Return(txQ).Maybe()
	sagaRepo.EXPECT().Conn(testifymock.Anything).Return(connQ).Maybe()

	notFound := func() error { return cerr.ResourceNotFound(nil) }

	txQ.EXPECT().Create(
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
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).RunAndReturn(func(
		_ context.Context,
		sagaType model.SagaType,
		version model.SagaDefinitionVersion,
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
		store.mu.Lock()
		defer store.mu.Unlock()

		key := store.mapKey(sagaType, businessKey)
		if existing, ok := store.byKey[key]; ok {
			return store.instances[existing], repo.ErrSagaAlreadyExists
		}

		id := model.SagaID(uuid.New())
		instance := model.SagaInstance{
			ID:                id,
			Type:              sagaType,
			DefinitionVersion: version,
			BusinessKey:       businessKey,
			SubjectType:       subjectType,
			SubjectID:         subjectID,
			Phase:             phase,
			Status:            status,
			CurrentStep:       currentStep,
			CorrelationID:     correlationID,
			CausationID:       causationID,
			CreatedAt:         createdAt,
			UpdatedAt:         updatedAt,
			Version:           1,
		}
		store.instances[id] = instance
		store.byKey[key] = id
		return instance, nil
	}).Maybe()

	txQ.EXPECT().Update(
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
	).RunAndReturn(func(
		_ context.Context,
		id model.SagaID,
		expectedVersion int64,
		phase model.SagaPhase,
		status model.SagaStatus,
		currentStep model.SagaStep,
		stepAttemptCount int,
		nextAttemptAt *time.Time,
		_ *string,
		_ *time.Time,
		updatedAt time.Time,
	) (model.SagaInstance, error) {
		store.mu.Lock()
		defer store.mu.Unlock()

		current, ok := store.instances[id]
		if !ok || current.Version != expectedVersion {
			return model.SagaInstance{}, repo.ErrSagaVersionConflict
		}

		current.Phase = phase
		current.Status = status
		current.CurrentStep = currentStep
		current.StepAttemptCount = stepAttemptCount
		current.NextAttemptAt = nextAttemptAt
		current.UpdatedAt = updatedAt
		current.Version++
		store.instances[id] = current
		return current, nil
	}).Maybe()

	txQ.EXPECT().RecordForwardStepCompletion(
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
	).Return(model.SagaForwardStepCompletion{}, nil).Maybe()

	lookupByID := func(_ context.Context, id model.SagaID) (model.SagaInstance, error) {
		store.mu.Lock()
		defer store.mu.Unlock()

		instance, ok := store.instances[id]
		if !ok {
			return model.SagaInstance{}, notFound()
		}

		return instance, nil
	}
	lookupByKey := func(
		_ context.Context,
		sagaType model.SagaType,
		businessKey model.SagaBusinessKey,
	) (model.SagaInstance, error) {
		store.mu.Lock()
		defer store.mu.Unlock()

		id, ok := store.byKey[store.mapKey(sagaType, businessKey)]
		if !ok {
			return model.SagaInstance{}, notFound()
		}

		return store.instances[id], nil
	}

	connQ.EXPECT().GetByID(testifymock.Anything, testifymock.Anything).RunAndReturn(lookupByID).Maybe()
	connQ.EXPECT().GetByTypeAndBusinessKey(testifymock.Anything, testifymock.Anything, testifymock.Anything).
		RunAndReturn(lookupByKey).Maybe()
	txQ.EXPECT().GetByTypeAndBusinessKey(testifymock.Anything, testifymock.Anything, testifymock.Anything).
		RunAndReturn(lookupByKey).Maybe()
	txQ.EXPECT().GetByID(testifymock.Anything, testifymock.Anything).RunAndReturn(lookupByID).Maybe()
	// OTP continuation locks the Saga row after lookup (canonical Saga→…→CS order).
	txQ.EXPECT().LockByID(testifymock.Anything, testifymock.Anything).RunAndReturn(lookupByID).Maybe()

	// Collapsed prepare path loads the bound customer's external identity.
	h.customerCQ.EXPECT().GetByID(testifymock.Anything, h.customerID).
		Return(h.customerProfile(), nil).Maybe()

	def, err := NewConfirmationSagaDefinition(
		h.pool,
		h.uc.checkoutSession,
		h.uc.customerProfile,
		h.uc.contract,
		h.uc.paymentAttempt,
		sagaRepo,
		h.contracts,
		h.uc.lifecycle,
		h.uc.clock,
		time.Second,
		time.Minute,
		8,
		ConfirmationSagaHooks{},
	)
	if err != nil {
		h.t.Fatalf("new confirmation saga definition: %v", err)
	}

	h.uc.confirmationSagaRepo = sagaRepo
	h.uc.confirmationSagaExecutor = executor
	h.uc.confirmationSagaDefinition = def
	executor.uc = h.uc
}

// expectConfirmClaimStepLock expects LockByID after MarkConfirming for the
// in-TX checkout_confirm_claimed step.
func (h *harness) expectConfirmClaimStepLock(_ model.CheckoutSession) {
	h.t.Helper()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).RunAndReturn(
		func(context.Context, model.CheckoutSessionID) (model.CheckoutSession, error) {
			return h.confirmingSession(), nil
		},
	).Once()
}

// expectConfirmSagaDriveLock expects LockByID at the start of the collapsed
// harness saga executor.
func (h *harness) expectConfirmSagaDriveLock(_ model.CheckoutSession) {
	h.t.Helper()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).RunAndReturn(
		func(context.Context, model.CheckoutSessionID) (model.CheckoutSession, error) {
			return h.confirmingSession(), nil
		},
	).Once()
}

// expectConfirmProjectionLock expects LockByID after saga drive for HTTP
// response projection.
func (h *harness) expectConfirmProjectionLock(session model.CheckoutSession) {
	h.t.Helper()
	h.sessionTx.EXPECT().LockByID(h.ctx, h.sessionID).Return(session, nil).Once()
}
