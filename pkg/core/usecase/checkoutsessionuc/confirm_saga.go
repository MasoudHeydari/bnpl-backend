package checkoutsessionuc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/event/checkoutlifecycle"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

const (
	checkoutConfirmationSagaType    model.SagaType              = "checkout_confirmation"
	checkoutConfirmationSagaVersion model.SagaDefinitionVersion = 1

	checkoutConfirmClaimedStep     model.SagaStep = "checkout_confirm_claimed"
	checkoutContractPreparedStep   model.SagaStep = "checkout_contract_prepared"
	checkoutSessionLinkedStep      model.SagaStep = "checkout_session_linked"
	checkoutFCFulfillmentStep      model.SagaStep = "checkout_fc_fulfillment"
	checkoutPaymentSubmittedStep   model.SagaStep = "checkout_payment_submitted"
	checkoutMerchantRecognizedStep model.SagaStep = "checkout_merchant_recognized"

	checkoutConfirmationSagaSubjectType = "checkout_session"
)

// ConfirmationSagaHooks are optional test barriers. Production leaves all nil.
type ConfirmationSagaHooks struct {
	BeforePrepareContract func(context.Context) error
	AfterPrepareContract  func(context.Context) error
	BeforeLink            func(context.Context) error
	BeforeFCSubmit        func(context.Context) error

	// BeforePaymentSubmitted runs inside the payment_submitted transactional
	// step after the Saga row is locked and before CheckoutSession is locked.
	// Tests use it to interleave OTP continuation (which must lock Saga first)
	// against an in-flight Saga step without relying on sleeps.
	BeforePaymentSubmitted func(context.Context) error

	// OnResumeConfirming runs when a second Confirm observes an unlinked
	// confirming session and is about to resume its Saga. It runs outside the
	// session lock so tests can overlap that resume with compensation.
	OnResumeConfirming func(context.Context) error
}

// ConfirmationSagaSettings configures business-specific retry timing.
type ConfirmationSagaSettings struct {
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
}

// ConfirmationSagaDefinition describes durable checkout confirmation.
//
// MarkConfirming and Saga creation commit together in the start transaction.
// Contract prepare and Financial Core submission run as ExternalSteps so no
// database transaction holds locks across network I/O. Initial-payment
// activation remains on the authoritative payment-result path after
// checkout_payment_submitted.
type ConfirmationSagaDefinition struct {
	pool             repo.Pool
	checkoutSession  repo.CheckoutSession
	customerProfile  repo.CustomerProfile
	contract         repo.Contract
	paymentAttempt   repo.PaymentAttempt
	sagas            repo.Saga
	contractUC       contractCreator
	lifecycle        *LifecycleOutbox
	clock            clock.Clock
	retryBaseDelay   time.Duration
	retryMaxDelay    time.Duration
	maxRetryAttempts int
	hooks            ConfirmationSagaHooks

	claimedStep      *confirmClaimedStep
	prepareStep      *confirmContractPreparedStep
	linkStep         *confirmSessionLinkedStep
	fcStep           *confirmFCFulfillmentStep
	paymentDoneStep  *confirmPaymentSubmittedStep
	merchantDoneStep *confirmMerchantRecognizedStep
}

var _ saga.Definition = (*ConfirmationSagaDefinition)(nil)

// NewConfirmationSagaDefinition constructs the checkout confirmation Saga.
//
// maxRetryAttempts is the number of additional retries after the initial
// execution of a retryable step (default 8 → up to 9 StepAttemptCount claims).
func NewConfirmationSagaDefinition(
	pool repo.Pool,
	checkoutSession repo.CheckoutSession,
	customerProfile repo.CustomerProfile,
	contract repo.Contract,
	paymentAttempt repo.PaymentAttempt,
	sagas repo.Saga,
	contractUC contractCreator,
	lifecycle *LifecycleOutbox,
	clk clock.Clock,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	maxRetryAttempts int,
	hooks ConfirmationSagaHooks,
) (*ConfirmationSagaDefinition, error) {
	const op = "checkoutsessionuc.NewConfirmationSagaDefinition"

	if pool == nil {
		return nil, fmt.Errorf("%s: pool is required", op)
	}

	if checkoutSession == nil {
		return nil, fmt.Errorf("%s: checkout session repository is required", op)
	}

	if customerProfile == nil {
		return nil, fmt.Errorf("%s: customer profile repository is required", op)
	}

	if contract == nil {
		return nil, fmt.Errorf("%s: contract repository is required", op)
	}

	if paymentAttempt == nil {
		return nil, fmt.Errorf("%s: payment attempt repository is required", op)
	}

	if sagas == nil {
		return nil, fmt.Errorf("%s: saga repository is required", op)
	}

	if contractUC == nil {
		return nil, fmt.Errorf("%s: contract use case is required", op)
	}

	if lifecycle == nil {
		return nil, fmt.Errorf("%s: lifecycle outbox is required", op)
	}

	if clk == nil {
		return nil, fmt.Errorf("%s: clock is required", op)
	}

	if retryBaseDelay <= 0 {
		return nil, fmt.Errorf("%s: confirmation saga retry base delay must be positive", op)
	}

	if retryMaxDelay < retryBaseDelay {
		return nil, fmt.Errorf("%s: confirmation saga retry max delay must be >= retry base delay", op)
	}

	if maxRetryAttempts <= 0 {
		return nil, fmt.Errorf("%s: confirmation saga max retry attempts must be positive", op)
	}

	d := &ConfirmationSagaDefinition{
		pool:             pool,
		checkoutSession:  checkoutSession,
		customerProfile:  customerProfile,
		contract:         contract,
		paymentAttempt:   paymentAttempt,
		sagas:            sagas,
		contractUC:       contractUC,
		lifecycle:        lifecycle,
		clock:            clk,
		retryBaseDelay:   retryBaseDelay,
		retryMaxDelay:    retryMaxDelay,
		maxRetryAttempts: maxRetryAttempts,
		hooks:            hooks,
	}
	d.claimedStep = &confirmClaimedStep{def: d}
	d.prepareStep = &confirmContractPreparedStep{def: d}
	d.linkStep = &confirmSessionLinkedStep{def: d}
	d.fcStep = &confirmFCFulfillmentStep{def: d}
	d.paymentDoneStep = &confirmPaymentSubmittedStep{def: d}
	d.merchantDoneStep = &confirmMerchantRecognizedStep{def: d}

	return d, nil
}

func (d *ConfirmationSagaDefinition) Type() model.SagaType { return checkoutConfirmationSagaType }

func (d *ConfirmationSagaDefinition) Version() model.SagaDefinitionVersion {
	return checkoutConfirmationSagaVersion
}

func (d *ConfirmationSagaDefinition) InitialStep() model.SagaStep {
	return checkoutConfirmClaimedStep
}

func (d *ConfirmationSagaDefinition) IsForwardTerminalStep(step model.SagaStep) bool {
	return step == checkoutPaymentSubmittedStep || step == checkoutMerchantRecognizedStep
}

func (d *ConfirmationSagaDefinition) Step(name model.SagaStep) (saga.Step, bool) {
	switch name {
	case checkoutConfirmClaimedStep:
		return d.claimedStep, true

	case checkoutContractPreparedStep:
		return d.prepareStep, true

	case checkoutSessionLinkedStep:
		return d.linkStep, true

	case checkoutFCFulfillmentStep:
		return d.fcStep, true

	case checkoutPaymentSubmittedStep:
		return d.paymentDoneStep, true

	case checkoutMerchantRecognizedStep:
		return d.merchantDoneStep, true

	default:
		return nil, false
	}
}

func (d *ConfirmationSagaDefinition) CompensationFor(
	completedStep model.SagaStep,
) (saga.Compensation, bool) {
	switch completedStep {
	case checkoutConfirmClaimedStep:
		return saga.Compensation{Mode: saga.CompensationModeDirect}, true

	case checkoutContractPreparedStep,
		checkoutSessionLinkedStep,
		checkoutFCFulfillmentStep,
		checkoutPaymentSubmittedStep,
		checkoutMerchantRecognizedStep:
		return saga.Compensation{Mode: saga.CompensationModeNone}, true

	default:
		return saga.Compensation{}, false
	}
}

func (d *ConfirmationSagaDefinition) CanTransition(
	phase model.SagaPhase,
	from model.SagaStep,
	to model.SagaStep,
) bool {
	if phase != model.ForwardSagaPhase {
		return false
	}

	switch from {
	case checkoutConfirmClaimedStep:
		return to == checkoutContractPreparedStep

	case checkoutContractPreparedStep:
		return to == checkoutSessionLinkedStep

	case checkoutSessionLinkedStep:
		return to == checkoutFCFulfillmentStep

	case checkoutFCFulfillmentStep:
		return to == checkoutPaymentSubmittedStep || to == checkoutMerchantRecognizedStep

	default:
		return false
	}
}

func confirmationSagaSessionID(instance model.SagaInstance) (model.CheckoutSessionID, error) {
	const op = "checkoutsessionuc.confirmationSagaSessionID"

	if instance.SubjectType == nil || strings.TrimSpace(*instance.SubjectType) != checkoutConfirmationSagaSubjectType {
		return model.CheckoutSessionID{}, fmt.Errorf("%s: invalid subject type", op)
	}

	if instance.SubjectID == nil {
		return model.CheckoutSessionID{}, fmt.Errorf("%s: subject id is required", op)
	}

	sessionID, err := model.ParseCheckoutSessionID(strings.TrimSpace(*instance.SubjectID))
	if err != nil {
		return model.CheckoutSessionID{}, fmt.Errorf("%s: invalid subject id: %w", op, err)
	}

	if sessionID.IsZero() {
		return model.CheckoutSessionID{}, fmt.Errorf("%s: subject id must not be zero", op)
	}

	return sessionID, nil
}

func confirmationSagaRetryDelay(attemptCount int, base, max time.Duration) time.Duration {
	if attemptCount < 1 {
		attemptCount = 1
	}

	delay := base
	for i := 1; i < attemptCount; i++ {
		if delay >= max/2 {
			return max
		}

		delay *= 2
	}

	if delay > max {
		return max
	}

	return delay
}

func boundedConfirmationSagaReason(value string) string {
	const maxReasonLength = 2048

	value = strings.TrimSpace(value)
	if value == "" {
		value = "unknown confirmation saga failure"
	}

	if len(value) > maxReasonLength {
		value = value[:maxReasonLength]
	}

	return value
}

func (d *ConfirmationSagaDefinition) loadSession(
	ctx context.Context,
	instance model.SagaInstance,
) (model.CheckoutSession, error) {
	sessionID, err := confirmationSagaSessionID(instance)
	if err != nil {
		return model.CheckoutSession{}, err
	}

	var session model.CheckoutSession
	err = d.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		sessionQ := d.checkoutSession.Conn(conn)
		row, getErr := sessionQ.GetByID(ctx, sessionID)
		if getErr != nil {
			return getErr
		}

		session = row
		return nil
	})

	return session, err
}

func (d *ConfirmationSagaDefinition) loadExternalCustomerID(
	ctx context.Context,
	session model.CheckoutSession,
) (model.ExternalCustomerID, error) {
	const op = "checkoutsessionuc.ConfirmationSagaDefinition.loadExternalCustomerID"

	if session.CustomerID == nil || session.CustomerID.IsZero() {
		return model.ExternalCustomerID{}, fmt.Errorf("%s: session has no bound customer", op)
	}

	var profile model.CustomerProfile
	err := d.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		customerQ := d.customerProfile.Conn(conn)
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

// assertSagaClaimCurrent rejects a worker whose Saga version is no longer the
// version it claimed. The check runs immediately before an external mutation
// so a stale worker does not start a second Contract or Financial Core call
// after another worker has advanced the Saga.
//
// TODO(saga-fencing): There is a TOCTOU window between this version check and
// the later commit of business mutations (Contract prepare, CheckoutSession
// link, PaymentAttempt writes, Financial Core RPCs). Saga CAS fences only the
// SagaInstance row. It does not atomically fence Contract, PaymentAttempt, or
// Financial Core effects. Safe convergence must continue to rely on
// ConfirmationAttempt identity, Decision uniqueness, domain invariants, and
// Financial Core idempotency — not on inventing a general-purpose fencing
// framework here.
func (d *ConfirmationSagaDefinition) assertSagaClaimCurrent(
	ctx context.Context,
	instance model.SagaInstance,
) error {
	const op = "checkoutsessionuc.ConfirmationSagaDefinition.assertSagaClaimCurrent"

	var current model.SagaInstance
	err := d.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		sagasQ := d.sagas.Conn(conn)
		row, getErr := sagasQ.GetByID(ctx, instance.ID)
		if getErr != nil {
			return getErr
		}

		current = row
		return nil
	})
	if err != nil {
		return err
	}

	if current.Version != instance.Version {
		return fmt.Errorf(
			"%s: %w: confirmation saga %s claim version %d is stale; current version %d",
			op,
			repo.ErrSagaVersionConflict,
			instance.ID,
			instance.Version,
			current.Version,
		)
	}

	return nil
}

// externalStepError persists a durable Saga outcome for failures that occur
// before an external step can return its own StepResult. A stale claim stays a
// raw version conflict so the driver stops. Caller cancellation, deadlines, and
// other classified failures become retry, reconciliation, or manual review
// instead of an unpersisted error that holds the lease until it expires.
func (d *ConfirmationSagaDefinition) externalStepError(
	err error,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	if err == nil {
		return saga.StepResult{}, nil
	}

	if errors.Is(err, repo.ErrSagaVersionConflict) {
		return saga.StepResult{}, err
	}

	now := d.clock.Now().UTC()
	if classified, ok := classifyConfirmationExternalError(
		err,
		instance.StepAttemptCount,
		d.maxRetryAttempts,
		d.retryBaseDelay,
		d.retryMaxDelay,
		now,
	); ok {
		return classified, nil
	}

	return saga.StepResult{}, err
}

type confirmClaimedStep struct{ def *ConfirmationSagaDefinition }

var (
	_ saga.TransactionalStep              = (*confirmClaimedStep)(nil)
	_ saga.TransactionalCompensatableStep = (*confirmClaimedStep)(nil)
)

func (s *confirmClaimedStep) Name() model.SagaStep { return checkoutConfirmClaimedStep }

func (s *confirmClaimedStep) ExecuteTx(
	ctx context.Context,
	tx repo.Tx,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	const op = "checkoutsessionuc.confirmClaimedStep.ExecuteTx"

	sessionID, err := confirmationSagaSessionID(instance)
	if err != nil {
		return saga.StepResult{}, err
	}

	sessionQ := s.def.checkoutSession.Tx(tx)
	session, err := sessionQ.LockByID(ctx, sessionID)
	if err != nil {
		return saga.StepResult{}, err
	}

	if session.Status != model.ConfirmingCheckoutSessionStatus {
		return saga.StepResult{}, fmt.Errorf(
			"%s: expected confirming session %s, got %s", op, sessionID, session.Status,
		)
	}

	if session.ContractID != nil && !session.ContractID.IsZero() {
		return saga.StepResult{}, fmt.Errorf("%s: confirming session %s already has a contract", op, sessionID)
	}

	if mismatch, ok := sessionMatchesConfirmationAttempt(session, instance); !ok {
		return mismatch, nil
	}

	next := checkoutContractPreparedStep
	return saga.StepResult{Outcome: saga.StepOutcomeCompleted, NextStep: &next}, nil
}

func (s *confirmClaimedStep) CompensateTx(
	ctx context.Context,
	tx repo.Tx,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	const op = "checkoutsessionuc.confirmClaimedStep.CompensateTx"

	sessionID, err := confirmationSagaSessionID(instance)
	if err != nil {
		return saga.StepResult{}, err
	}

	sessionQ := s.def.checkoutSession.Tx(tx)
	session, err := sessionQ.LockByID(ctx, sessionID)
	if err != nil {
		return saga.StepResult{}, err
	}

	switch session.Status {
	case model.OpenCheckoutSessionStatus:
		return saga.StepResult{Outcome: saga.StepOutcomeCompleted}, nil

	case model.ConfirmingCheckoutSessionStatus:
		if session.ContractID != nil && !session.ContractID.IsZero() {
			reason := fmt.Sprintf("session %s already linked a contract before claim compensation", sessionID)
			return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, nil
		}

		if session.DecisionID == nil || session.DecisionID.IsZero() {
			reason := fmt.Sprintf("session %s has no decision during claim compensation", sessionID)
			return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, nil
		}

		contractQ := s.def.contract.Tx(tx)
		_, findErr := contractQ.GetByDecisionID(ctx, *session.DecisionID)
		if findErr == nil {
			reason := fmt.Sprintf("decision %s already has a contract; refuse claim reopen", session.DecisionID)
			return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, nil
		}

		if !cerr.IsCode(findErr, cerr.CodeResourceNotFound) {
			return saga.StepResult{}, findErr
		}

		attemptID, attemptErr := confirmationAttemptFromSagaBusinessKey(instance)
		if attemptErr != nil {
			reason := boundedConfirmationSagaReason(attemptErr.Error())
			return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, nil
		}

		if session.ConfirmationAttemptID == nil || *session.ConfirmationAttemptID != attemptID {
			reason := boundedConfirmationSagaReason(
				fmt.Sprintf("session %s attempt mismatch during claim compensation", sessionID),
			)
			return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, nil
		}

		now := s.def.clock.Now().UTC()
		if _, err := sessionQ.ReopenConfirmingToOpen(ctx, session.ID, attemptID, now); err != nil {
			if cerr.IsCode(err, cerr.CodeConflict) {
				reason := boundedConfirmationSagaReason(err.Error())
				return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, nil
			}

			return saga.StepResult{}, err
		}

		return saga.StepResult{Outcome: saga.StepOutcomeCompleted}, nil

	default:
		reason := fmt.Sprintf("session %s moved to %s while claim compensation pending", sessionID, session.Status)
		return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, nil
	}
}

type confirmContractPreparedStep struct{ def *ConfirmationSagaDefinition }

var _ saga.ExternalStep = (*confirmContractPreparedStep)(nil)

func (s *confirmContractPreparedStep) Name() model.SagaStep { return checkoutContractPreparedStep }

func (s *confirmContractPreparedStep) Execute(
	ctx context.Context,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	const op = "checkoutsessionuc.confirmContractPreparedStep.Execute"

	if hook := s.def.hooks.BeforePrepareContract; hook != nil {
		if err := hook(ctx); err != nil {
			return s.def.externalStepError(err, instance)
		}
	}

	// Re-check the claim after the barrier. A worker whose lease expired must
	// not prepare a Contract for a Saga version another worker already advanced.
	if err := s.def.assertSagaClaimCurrent(ctx, instance); err != nil {
		return s.def.externalStepError(err, instance)
	}

	session, err := s.def.loadSession(ctx, instance)
	if err != nil {
		return s.def.externalStepError(err, instance)
	}

	if mismatch, ok := sessionMatchesConfirmationAttempt(session, instance); !ok {
		return mismatch, nil
	}

	if session.DecisionID == nil || session.DecisionID.IsZero() {
		return s.def.externalStepError(fmt.Errorf("%s: session has no decision", op), instance)
	}

	// Idempotent resume: if a Contract already exists for the Decision, advance.
	var existing model.Contract
	findErr := s.def.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		contractQ := s.def.contract.Conn(conn)
		row, getErr := contractQ.GetByDecisionID(ctx, *session.DecisionID)
		if getErr != nil {
			return getErr
		}

		existing = row
		return nil
	})
	if findErr == nil && !existing.ID.IsZero() {
		next := checkoutSessionLinkedStep
		return saga.StepResult{Outcome: saga.StepOutcomeCompleted, NextStep: &next}, nil
	}

	if findErr != nil && !cerr.IsCode(findErr, cerr.CodeResourceNotFound) {
		return s.def.externalStepError(findErr, instance)
	}

	externalCustomerID, err := s.def.loadExternalCustomerID(ctx, session)
	if err != nil {
		return s.def.externalStepError(err, instance)
	}

	_, err = s.def.contractUC.PrepareCheckoutContract(ctx, contractuc.CreateContractInput{
		DecisionID:         *session.DecisionID,
		ExternalCustomerID: externalCustomerID,
	})
	if err != nil {
		// Prepare is convergent on DecisionID; a lost response may already have
		// committed a Contract. Retry only failures that a repeat can resolve.
		// Deterministic pre-contract rejections compensate instead.
		now := s.def.clock.Now().UTC()
		if classified, ok := classifyConfirmationPrepareError(
			err,
			instance.StepAttemptCount,
			s.def.maxRetryAttempts,
			s.def.retryBaseDelay,
			s.def.retryMaxDelay,
			now,
		); ok {
			return classified, nil
		}

		return s.def.externalStepError(err, instance)
	}

	if hook := s.def.hooks.AfterPrepareContract; hook != nil {
		if err := hook(ctx); err != nil {
			return s.def.externalStepError(err, instance)
		}
	}

	next := checkoutSessionLinkedStep
	return saga.StepResult{Outcome: saga.StepOutcomeCompleted, NextStep: &next}, nil
}

type confirmSessionLinkedStep struct{ def *ConfirmationSagaDefinition }

var _ saga.TransactionalStep = (*confirmSessionLinkedStep)(nil)

func (s *confirmSessionLinkedStep) Name() model.SagaStep { return checkoutSessionLinkedStep }

func (s *confirmSessionLinkedStep) ExecuteTx(
	ctx context.Context,
	tx repo.Tx,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	const op = "checkoutsessionuc.confirmSessionLinkedStep.ExecuteTx"

	if hook := s.def.hooks.BeforeLink; hook != nil {
		if err := hook(ctx); err != nil {
			return saga.StepResult{}, err
		}
	}

	sessionID, err := confirmationSagaSessionID(instance)
	if err != nil {
		return saga.StepResult{}, err
	}

	sessionQ := s.def.checkoutSession.Tx(tx)
	session, err := sessionQ.LockByID(ctx, sessionID)
	if err != nil {
		return saga.StepResult{}, err
	}

	if mismatch, ok := sessionMatchesConfirmationAttempt(session, instance); !ok {
		return mismatch, nil
	}

	if session.DecisionID == nil || session.DecisionID.IsZero() {
		return saga.StepResult{}, fmt.Errorf("%s: session has no decision", op)
	}

	contractQ := s.def.contract.Tx(tx)
	contractRow, err := contractQ.GetByDecisionID(ctx, *session.DecisionID)
	if err != nil {
		return saga.StepResult{}, err
	}

	now := s.def.clock.Now().UTC()
	attached := session
	alreadyLinked := session.ContractID != nil && *session.ContractID == contractRow.ID

	if !alreadyLinked {
		if session.Status != model.ConfirmingCheckoutSessionStatus &&
			session.Status != model.ProcessingCheckoutSessionStatus &&
			session.Status != model.CompletedCheckoutSessionStatus {
			return saga.StepResult{}, fmt.Errorf("%s: session status is %q", op, session.Status)
		}

		attached, err = sessionQ.LinkContract(ctx, session.ID, contractRow.ID, now)
		if err != nil {
			return saga.StepResult{}, err
		}
	}

	if attached.Status != model.ProcessingCheckoutSessionStatus &&
		attached.Status != model.CompletedCheckoutSessionStatus {
		if transitionErr := attached.Status.ValidateTransitionTo(model.ProcessingCheckoutSessionStatus); transitionErr != nil {
			return saga.StepResult{}, cerr.InternalError(fmt.Errorf("%s: %w", op, transitionErr))
		}

		if _, err := sessionQ.UpdateStatus(ctx, attached.ID, model.ProcessingCheckoutSessionStatus, now); err != nil {
			return saga.StepResult{}, err
		}
	}

	next := checkoutFCFulfillmentStep
	return saga.StepResult{Outcome: saga.StepOutcomeCompleted, NextStep: &next}, nil
}

type confirmFCFulfillmentStep struct{ def *ConfirmationSagaDefinition }

var _ saga.ExternalStep = (*confirmFCFulfillmentStep)(nil)

func (s *confirmFCFulfillmentStep) Name() model.SagaStep { return checkoutFCFulfillmentStep }

func (s *confirmFCFulfillmentStep) Execute(
	ctx context.Context,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	const op = "checkoutsessionuc.confirmFCFulfillmentStep.Execute"

	if hook := s.def.hooks.BeforeFCSubmit; hook != nil {
		if err := hook(ctx); err != nil {
			return s.def.externalStepError(err, instance)
		}
	}

	if err := s.def.assertSagaClaimCurrent(ctx, instance); err != nil {
		return s.def.externalStepError(err, instance)
	}

	session, err := s.def.loadSession(ctx, instance)
	if err != nil {
		return s.def.externalStepError(err, instance)
	}

	if mismatch, ok := sessionMatchesConfirmationAttempt(session, instance); !ok {
		return mismatch, nil
	}

	if session.ContractID == nil || session.ContractID.IsZero() {
		return s.def.externalStepError(fmt.Errorf("%s: session has no linked contract", op), instance)
	}

	var (
		contractRow     model.Contract
		paymentRequired bool
		attempt         model.PaymentAttempt
	)

	err = s.def.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		contractQ := s.def.contract.Conn(conn)
		row, getErr := contractQ.GetByID(ctx, *session.ContractID)
		if getErr != nil {
			return getErr
		}

		contractRow = row
		return nil
	})
	if err != nil {
		return s.def.externalStepError(err, instance)
	}

	// Derive payment requirement from whether a checkout PaymentAttempt exists.
	attemptErr := s.def.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		return conn.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			paymentAttemptQ := s.def.paymentAttempt.Tx(tx)
			row, getErr := paymentAttemptQ.GetByContractIDAndPurposeWithItems(
				ctx,
				contractRow.ID,
				model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
			)
			if getErr != nil {
				return getErr
			}

			attempt = row
			return nil
		})
	})
	switch {
	case attemptErr == nil:
		paymentRequired = true

	case cerr.IsCode(attemptErr, cerr.CodeResourceNotFound):
		paymentRequired = false

	default:
		return s.def.externalStepError(attemptErr, instance)
	}

	prepared := contractuc.CreateContractOutput{
		Contract:        contractRow,
		PaymentRequired: paymentRequired,
		PaymentAttempt:  attempt,
	}

	_, err = s.def.contractUC.SubmitPreparedCheckoutFulfillment(ctx, prepared)
	if err != nil {
		now := s.def.clock.Now().UTC()
		if classified, ok := classifyConfirmationExternalError(
			err,
			instance.StepAttemptCount,
			s.def.maxRetryAttempts,
			s.def.retryBaseDelay,
			s.def.retryMaxDelay,
			now,
		); ok {
			return classified, nil
		}

		return s.def.externalStepError(err, instance)
	}

	if paymentRequired {
		next := checkoutPaymentSubmittedStep
		return saga.StepResult{Outcome: saga.StepOutcomeCompleted, NextStep: &next}, nil
	}

	next := checkoutMerchantRecognizedStep
	return saga.StepResult{Outcome: saga.StepOutcomeCompleted, NextStep: &next}, nil
}

type confirmPaymentSubmittedStep struct{ def *ConfirmationSagaDefinition }

var _ saga.TransactionalStep = (*confirmPaymentSubmittedStep)(nil)

func (s *confirmPaymentSubmittedStep) Name() model.SagaStep { return checkoutPaymentSubmittedStep }

func (s *confirmPaymentSubmittedStep) ExecuteTx(
	ctx context.Context,
	tx repo.Tx,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	const op = "checkoutsessionuc.confirmPaymentSubmittedStep.ExecuteTx"

	if hook := s.def.hooks.BeforePaymentSubmitted; hook != nil {
		if err := hook(ctx); err != nil {
			return saga.StepResult{}, err
		}
	}

	sessionID, err := confirmationSagaSessionID(instance)
	if err != nil {
		return saga.StepResult{}, err
	}

	sessionQ := s.def.checkoutSession.Tx(tx)
	session, err := sessionQ.LockByID(ctx, sessionID)
	if err != nil {
		return saga.StepResult{}, err
	}

	if mismatch, ok := sessionMatchesConfirmationAttempt(session, instance); !ok {
		return mismatch, nil
	}

	if session.ContractID == nil || session.ContractID.IsZero() {
		return saga.StepResult{}, fmt.Errorf("%s: session has no linked contract", op)
	}

	if session.Status != model.ProcessingCheckoutSessionStatus &&
		session.Status != model.CompletedCheckoutSessionStatus {
		reason := fmt.Sprintf("session %s is %s after payment submit", sessionID, session.Status)
		return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, nil
	}

	// Terminal for the Saga: payment-result processing owns activation.
	return saga.StepResult{Outcome: saga.StepOutcomeCompleted}, nil
}

type confirmMerchantRecognizedStep struct{ def *ConfirmationSagaDefinition }

var _ saga.TransactionalStep = (*confirmMerchantRecognizedStep)(nil)

func (s *confirmMerchantRecognizedStep) Name() model.SagaStep { return checkoutMerchantRecognizedStep }

func (s *confirmMerchantRecognizedStep) ExecuteTx(
	ctx context.Context,
	tx repo.Tx,
	instance model.SagaInstance,
) (saga.StepResult, error) {
	const op = "checkoutsessionuc.confirmMerchantRecognizedStep.ExecuteTx"

	sessionID, err := confirmationSagaSessionID(instance)
	if err != nil {
		return saga.StepResult{}, err
	}

	sessionQ := s.def.checkoutSession.Tx(tx)
	session, err := sessionQ.LockByID(ctx, sessionID)
	if err != nil {
		return saga.StepResult{}, err
	}

	if mismatch, ok := sessionMatchesConfirmationAttempt(session, instance); !ok {
		return mismatch, nil
	}

	if session.ContractID == nil || session.ContractID.IsZero() {
		return saga.StepResult{}, fmt.Errorf("%s: session has no linked contract", op)
	}

	contractQ := s.def.contract.Tx(tx)
	contractRow, err := contractQ.GetByID(ctx, *session.ContractID)
	if err != nil {
		return saga.StepResult{}, err
	}

	if contractRow.Status != model.ActiveContractStatus ||
		contractRow.FinancialMerchantEntitlementID == nil ||
		contractRow.FinancialReceivableID == nil ||
		*contractRow.FinancialMerchantEntitlementID == "" ||
		*contractRow.FinancialReceivableID == "" {
		reason := fmt.Sprintf("contract %s lacks durable Financial Core recognition", contractRow.ID)
		return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, nil
	}

	now := s.def.clock.Now().UTC()
	if err := finalizeRecognizedCheckoutTx(
		ctx,
		tx,
		s.def.checkoutSession,
		s.def.lifecycle,
		contractRow.ID,
		now,
	); err != nil {
		return saga.StepResult{}, fmt.Errorf("%s: finalize recognized checkout: %w", op, err)
	}

	return saga.StepResult{Outcome: saga.StepOutcomeCompleted}, nil
}

// finalizeRecognizedCheckoutTx marks the linked checkout completed after
// durable Financial Core recognition. Shared by the merchant-recognized Saga
// step and UseCase.ApplyPaymentSucceeded.
func finalizeRecognizedCheckoutTx(
	ctx context.Context,
	tx repo.Tx,
	checkoutSession repo.CheckoutSession,
	lifecycle *LifecycleOutbox,
	contractID model.ContractID,
	now time.Time,
) error {
	const op = "checkoutsessionuc.finalizeRecognizedCheckoutTx"

	sessionQ := checkoutSession.Tx(tx)

	locked, err := sessionQ.LockByContractID(ctx, contractID)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return nil
		}

		return fmt.Errorf("%s: lock checkout session by contract: %w", op, err)
	}

	switch locked.Status {
	case model.CompletedCheckoutSessionStatus:
		return nil

	case model.ProcessingCheckoutSessionStatus,
		model.ConfirmingCheckoutSessionStatus,
		model.OpenCheckoutSessionStatus:
		// Normal and recoverable paths into completed.

	default:
		return fmt.Errorf("%s: session %s status %s cannot complete", op, locked.ID, locked.Status)
	}

	if transitionErr := locked.Status.ValidateTransitionTo(model.CompletedCheckoutSessionStatus); transitionErr != nil {
		return fmt.Errorf("%s: %w", op, transitionErr)
	}

	updated, err := sessionQ.UpdateStatus(ctx, locked.ID, model.CompletedCheckoutSessionStatus, now)
	if err != nil {
		return err
	}

	return lifecycle.Enqueue(
		ctx,
		tx,
		updated,
		checkoutlifecycle.EventTypeCompleted,
		now,
	)
}
